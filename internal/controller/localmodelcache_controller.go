/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	labelLocalCache                  = "serving.ckodex.com/local-cache"
	labelNode                        = "serving.ckodex.com/node"
	labelModelHash                   = "serving.ckodex.com/model-hash"
	defaultCacheNamespace            = "default"
	cacheWorkloadNamespaceAnnotation = "serving.ckodex.com/cache-namespace"
	warmupJobPrefix                  = "lmc-warmup"
	localModelCacheRequeueInterval   = 30 * time.Second
)

// LocalModelCacheReconciler reconciles a LocalModelCache object.
type LocalModelCacheReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	APIReader client.Reader
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=localmodelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// IsNamespaceAllowed returns true when the namespace is permitted to reference this cache.
func IsNamespaceAllowed(lmc *servingv1alpha2.LocalModelCache, namespace string) bool {
	if len(lmc.Spec.AllowedNamespaces) == 0 {
		return true
	}
	for _, allowed := range lmc.Spec.AllowedNamespaces {
		if allowed == namespace || allowed == "*" {
			return true
		}
	}
	return false
}

func ModelURIHash(uri string) string {
	hash := sha256.Sum256([]byte(uri))
	return fmt.Sprintf("%x", hash[:8])
}

func PVCNameForNode(modelHash, nodeName string) string {
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))[:8]
	return fmt.Sprintf("lmc-%s-%s", modelHash, nodeHash)
}

func cacheWorkloadNamespace(lmc *servingv1alpha2.LocalModelCache) string {
	return defaultCacheNamespace
}

func (r *LocalModelCacheReconciler) resolveCacheWorkloadNamespace(ctx context.Context, lmc *servingv1alpha2.LocalModelCache) (string, error) {
	requested := lmc.Annotations[cacheWorkloadNamespaceAnnotation]
	if requested == "" {
		if err := validateWarmupStorageReferences(lmc); err != nil {
			return "", err
		}
		return cacheWorkloadNamespace(lmc), nil
	}
	if lmc.Labels[loraCacheManagedByLabel] != loraCacheManagedByAdapter {
		return "", fmt.Errorf("LocalModelCache %s has an untrusted cache namespace annotation", lmc.Name)
	}
	ownerNamespace := lmc.Annotations[loraCacheOwnerNamespace]
	ownerName := lmc.Annotations[loraCacheOwnerName]
	if ownerNamespace == "" || ownerName == "" || requested != ownerNamespace {
		return "", fmt.Errorf("LocalModelCache %s has incomplete or mismatched LoRA ownership metadata", lmc.Name)
	}
	owner := &servingv1alpha2.LLMLoraAdapter{}
	reader := r.cacheReader()
	if reader == nil {
		return "", fmt.Errorf("getting LocalModelCache %s LoRA owner %s/%s: no API reader configured", lmc.Name, ownerNamespace, ownerName)
	}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: ownerNamespace, Name: ownerName}, owner); err != nil {
		return "", fmt.Errorf("getting LocalModelCache %s LoRA owner %s/%s: %w", lmc.Name, ownerNamespace, ownerName, err)
	}
	if err := validateLoraCacheOwner(lmc, owner); err != nil {
		return "", err
	}
	if err := validateWarmupStorageReferences(lmc); err != nil {
		return "", err
	}
	return owner.Namespace, nil
}

func validateWarmupStorageReferences(lmc *servingv1alpha2.LocalModelCache) error {
	if lmc.Spec.Storage == nil {
		return nil
	}
	for kind, name := range map[string]string{
		"ServiceAccount": lmc.Spec.Storage.ServiceAccountName,
		"Secret":         lmc.Spec.Storage.SecretName,
	} {
		if strings.Contains(name, "/") {
			return fmt.Errorf("LocalModelCache %s %s reference must be namespaced to the resolved workload namespace", lmc.Name, kind)
		}
	}
	return nil
}

func (r *LocalModelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.APIReader = mgr.GetAPIReader()
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		For(&servingv1alpha2.LocalModelCache{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToLMC)).
		Complete(r)
}

func (r *LocalModelCacheReconciler) mapNodeToLMC(ctx context.Context, obj client.Object) []reconcile.Request {
	var lmcList servingv1alpha2.LocalModelCacheList
	if err := r.List(ctx, &lmcList); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(lmcList.Items))
	for _, lmc := range lmcList.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&lmc)})
	}
	return requests
}
