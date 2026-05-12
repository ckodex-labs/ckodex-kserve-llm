/*
Copyright 2026 CKodex Authors.
*/

package security

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ExternalSecretReconciler manages managed ExternalSecrets for model credentials.
// It uses Unstructured to provide non-blocking, opt-in support for external-secrets.io.
type ExternalSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// GVK for external-secrets.io/v1beta1 ExternalSecret
var ExternalSecretGVK = schema.GroupVersionKind{
	Group:   "external-secrets.io",
	Version: "v1beta1",
	Kind:    "ExternalSecret",
}

// ReconcileExternalSecret creates or updates an ExternalSecret if opted-in.
func (r *ExternalSecretReconciler) ReconcileExternalSecret(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "external-secret")

	storage := llmSvc.Spec.Model.Storage
	if storage == nil || storage.ExternalSecret == nil {
		return nil
	}

	// 1. Check if ExternalSecret GVK is available in the cluster.
	// This makes the integration non-blocking.
	exists, err := r.isGVKAvailable()
	if err != nil {
		return fmt.Errorf("check GVK availability: %w", err)
	}
	if !exists {
		logger.Info("external-secrets.io CRDs not found, skipping managed secret reconciliation")
		return nil
	}

	// 2. Build the Unstructured ExternalSecret
	spec := storage.ExternalSecret
	targetSecretName := llmSvc.Name + "-external-secret"

	es := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	es.SetGroupVersionKind(ExternalSecretGVK)
	// Some fake client versions need these explicitly in the map to avoid panics
	es.Object["apiVersion"] = ExternalSecretGVK.GroupVersion().String()
	es.Object["kind"] = ExternalSecretGVK.Kind
	
	es.SetName(llmSvc.Name)
	es.SetNamespace(llmSvc.Namespace)

	// Map data to ExternalSecret format
	dataArr := make([]interface{}, 0, len(spec.Data))
	for _, d := range spec.Data {
		dataArr = append(dataArr, map[string]interface{}{
			"secretKey": d.SecretKey,
			"remoteRef": map[string]interface{}{
				"key":      d.RemoteRef.Key,
				"property": d.RemoteRef.Property,
			},
		})
	}

	externalSecretSpec := make(map[string]interface{})
	externalSecretSpec["refreshInterval"] = spec.RefreshInterval
	externalSecretSpec["secretStoreRef"] = map[string]interface{}{
		"name": spec.SecretStoreRef.Name,
		"kind": spec.SecretStoreRef.Kind,
	}
	externalSecretSpec["target"] = map[string]interface{}{
		"name": targetSecretName,
	}
	externalSecretSpec["data"] = dataArr

	es.Object["spec"] = externalSecretSpec

	if err := controllerutil.SetControllerReference(llmSvc, es, r.Scheme); err != nil {
		return err
	}

	// 3. Create or Update
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ExternalSecretGVK)
	err = r.Get(ctx, client.ObjectKey{Name: es.GetName(), Namespace: es.GetNamespace()}, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("creating ExternalSecret", "name", es.GetName())
			return r.Create(ctx, es)
		}
		return err
	}

	// Update logic: for Unstructured we just overwrite the spec
	es.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("updating ExternalSecret", "name", es.GetName())
	return r.Update(ctx, es)
}

// isGVKAvailable checks if the ExternalSecret CRD is registered.
func (r *ExternalSecretReconciler) isGVKAvailable() (bool, error) {
	if r.Client == nil {
		return false, nil
	}

	// 1. Try RESTMapper (Cluster-aware)
	if mapper := r.RESTMapper(); mapper != nil {
		_, err := mapper.RESTMapping(ExternalSecretGVK.GroupKind(), ExternalSecretGVK.Version)
		if err == nil {
			return true, nil
		}
		if !meta.IsNoMatchError(err) {
			return false, err
		}
	}

	// 2. Fallback to Scheme (Environment-aware, useful for testing)
	if r.Scheme != nil && r.Scheme.Recognizes(ExternalSecretGVK) {
		return true, nil
	}

	return false, nil
}
