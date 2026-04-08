/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// AnnotationRegistrySecret is placed on a Namespace to name the Secret in the
	// operator's own namespace that holds the registry credentials to distribute.
	// Value: "<source-namespace>/<secret-name>", e.g. "ckodex-system/ghcr-pull-secret".
	AnnotationRegistrySecret = "ckodex.com/registry-secret"

	// pullSecretName is the name given to distributed pull secrets in tenant namespaces.
	pullSecretName = "ckodex-registry-pull"

	// serviceAccountDefault is patched to reference the pull secret.
	serviceAccountDefault = "default"
)

// ImagePullSecretReconciler watches Namespaces labeled ckodex.com/tenant-id and
// distributes image pull Secrets from the operator namespace into each tenant namespace.
//
// Distribution strategy:
//  1. A namespace opts in via the ckodex.com/registry-secret annotation pointing to
//     a source Secret in the operator namespace (e.g. "ckodex-system/ghcr-pull-secret").
//  2. The reconciler copies that Secret into the tenant namespace as "ckodex-registry-pull".
//  3. It patches the namespace's default ServiceAccount to reference the pull secret,
//     so all pods inherit it without explicit imagePullSecrets in every Pod spec.
//
// This avoids storing registry credentials in tenant namespaces as plaintext — the
// source Secret lives only in the operator namespace (which is tightly RBAC-controlled).
type ImagePullSecretReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string // namespace where source pull secrets reside
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;update;patch

// Reconcile distributes pull secrets into tenant namespaces.
func (r *ImagePullSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Name)

	var ns corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Only act on tenant namespaces.
	if _, ok := ns.Labels[LabelTenantID]; !ok {
		return ctrl.Result{}, nil
	}

	// Check for the registry-secret annotation.
	sourceRef, ok := ns.Annotations[AnnotationRegistrySecret]
	if !ok || sourceRef == "" {
		return ctrl.Result{}, nil
	}

	sourceNS, sourceName, err := parseSecretRef(sourceRef)
	if err != nil {
		logger.Error(err, "invalid registry-secret annotation", "value", sourceRef)
		return ctrl.Result{}, nil // don't requeue — annotation is malformed
	}

	// Fetch source secret from operator namespace.
	var source corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: sourceName, Namespace: sourceNS}, &source); err != nil {
		return ctrl.Result{}, fmt.Errorf("fetch source pull secret %s/%s: %w", sourceNS, sourceName, err)
	}

	// Distribute to tenant namespace.
	if err := r.ensurePullSecret(ctx, ns.Name, &source); err != nil {
		return ctrl.Result{}, fmt.Errorf("distribute pull secret to %s: %w", ns.Name, err)
	}

	// Patch default ServiceAccount to reference the pull secret.
	if err := r.patchServiceAccount(ctx, ns.Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch default SA in %s: %w", ns.Name, err)
	}

	logger.Info("image pull secret distributed")
	return ctrl.Result{}, nil
}

// ensurePullSecret creates or updates the pull secret in the target namespace.
// Only the .dockerconfigjson data is copied — metadata and labels are set by us.
func (r *ImagePullSecretReconciler) ensurePullSecret(ctx context.Context, targetNS string, source *corev1.Secret) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: targetNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				LabelTenantID:                  targetNS,
			},
			Annotations: map[string]string{
				"ckodex.com/source-secret": source.Namespace + "/" + source.Name,
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: source.Data[corev1.DockerConfigJsonKey],
		},
	}

	var existing corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Name: pullSecretName, Namespace: targetNS}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	// Update only if the dockerconfig data has changed.
	if string(existing.Data[corev1.DockerConfigJsonKey]) != string(desired.Data[corev1.DockerConfigJsonKey]) {
		existing.Data = desired.Data
		return r.Update(ctx, &existing)
	}
	return nil
}

// patchServiceAccount adds the pull secret reference to the default SA if not present.
func (r *ImagePullSecretReconciler) patchServiceAccount(ctx context.Context, namespace string) error {
	var sa corev1.ServiceAccount
	if err := r.Get(ctx, client.ObjectKey{Name: serviceAccountDefault, Namespace: namespace}, &sa); err != nil {
		return err
	}

	ref := corev1.LocalObjectReference{Name: pullSecretName}
	for _, existing := range sa.ImagePullSecrets {
		if existing.Name == pullSecretName {
			return nil // already present
		}
	}

	sa.ImagePullSecrets = append(sa.ImagePullSecrets, ref)

	patch, err := json.Marshal(map[string]interface{}{
		"imagePullSecrets": sa.ImagePullSecrets,
	})
	if err != nil {
		return err
	}

	return r.Patch(ctx, &sa, client.RawPatch(types.StrategicMergePatchType, patch))
}

// SetupWithManager registers the controller, filtered to tenant namespaces only.
func (r *ImagePullSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	tenantPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, hasTenant := e.Object.GetLabels()[LabelTenantID]
			_, hasAnnotation := e.Object.GetAnnotations()[AnnotationRegistrySecret]
			return hasTenant && hasAnnotation
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, hasTenant := e.ObjectNew.GetLabels()[LabelTenantID]
			_, hasAnnotation := e.ObjectNew.GetAnnotations()[AnnotationRegistrySecret]
			return hasTenant && hasAnnotation
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&corev1.Namespace{}).
		WithEventFilter(tenantPredicate).
		Named("image-pull-secret").
		Complete(r)
}

// parseSecretRef splits "namespace/name" into its components.
func parseSecretRef(ref string) (ns, name string, err error) {
	parts := splitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("registry-secret annotation must be in format <namespace>/<name>, got %q", ref)
	}
	return parts[0], parts[1], nil
}

func splitN(s, sep string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := indexOf(s, sep)
		if idx < 0 {
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return append(result, s)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
