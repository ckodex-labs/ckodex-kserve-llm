/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ToolSurfaceReconciler manages Istio configurations for FQDN-based egress isolation.
type ToolSurfaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ReconcileToolSurface ensures Istio Sidecar, PeerAuthentication, ServiceEntry,
// and DestinationRule resources are created for the LLMInferenceService.
func (r *ToolSurfaceReconciler) ReconcileToolSurface(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, activeLoras []servingv1alpha2.LLMLoraAdapter) error {
	logger := log.FromContext(ctx).WithValues("component", "tool-surface-reconciler")

	// 1. Aggregate all AllowedAPIs from foundation and adapters
	apis := make(map[string]bool)
	if llmSvc.Spec.ToolSurface != nil {
		for _, api := range llmSvc.Spec.ToolSurface.AllowedAPIs {
			apis[api] = true
		}
	}
	for _, lora := range activeLoras {
		if lora.Spec.ToolSurface != nil {
			for _, api := range lora.Spec.ToolSurface.AllowedAPIs {
				apis[api] = true
			}
		}
	}

	if len(apis) == 0 {
		return nil
	}

	// 2. PeerAuthentication (Enforce STRICT mTLS)
	if err := r.reconcilePeerAuthentication(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile PeerAuthentication: %w", err)
	}
	logger.V(1).Info("PeerAuthentication reconciled")

	// 3. Aggregate FQDNs for Sidecar and ServiceEntries
	var fqdns []string
	for f := range apis {
		fqdns = append(fqdns, f)
	}

	// 4. Sidecar (Restrict outbound scope)
	if err := r.reconcileIstioSidecar(ctx, llmSvc, fqdns); err != nil {
		return fmt.Errorf("reconcile Istio Sidecar: %w", err)
	}
	logger.V(1).Info("Istio Sidecar reconciled")

	// 5. Reconcile ServiceEntry + DestinationRule for each FQDN
	for fqdn := range apis {
		if err := r.reconcileIstioEgressResources(ctx, llmSvc, fqdn); err != nil {
			return fmt.Errorf("reconcile istio egress for %s: %w", fqdn, err)
		}
	}

	logger.Info("ToolSurface security perimeter reconciled", "allowed_apis", len(apis))
	return nil
}

func (r *ToolSurfaceReconciler) reconcileIstioEgressResources(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, fqdn string) error {
	// Resource name based on FQDN (sanitize for K8s)
	safeFqdn := strings.ReplaceAll(strings.ReplaceAll(fqdn, ".", "-"), "*", "wildcard")
	resourceName := fmt.Sprintf("%s-egress-%s", llmSvc.Name, safeFqdn)
	if len(resourceName) > 63 {
		resourceName = resourceName[:63]
	}

	// 1. ServiceEntry
	se := &unstructured.Unstructured{}
	se.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Kind:    "ServiceEntry",
		Version: "v1beta1",
	})
	se.SetName(resourceName)
	se.SetNamespace(llmSvc.Namespace)
	se.SetLabels(commonLabels(llmSvc))

	se.Object["spec"] = map[string]interface{}{
		"hosts":    []interface{}{fqdn},
		"location": "MESH_EXTERNAL",
		"ports": []interface{}{
			map[string]interface{}{
				"number":   443,
				"name":     "https",
				"protocol": "TLS",
			},
		},
		"resolution": "DNS",
	}

	if err := r.applyUnstructured(ctx, llmSvc, se); err != nil {
		return fmt.Errorf("apply ServiceEntry: %w", err)
	}

	// 2. DestinationRule (Enforce mTLS to egress gateway or just secure TLS)
	dr := &unstructured.Unstructured{}
	dr.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Kind:    "DestinationRule",
		Version: "v1beta1",
	})
	dr.SetName(resourceName)
	dr.SetNamespace(llmSvc.Namespace)
	dr.SetLabels(commonLabels(llmSvc))

	dr.Object["spec"] = map[string]interface{}{
		"host": fqdn,
		"trafficPolicy": map[string]interface{}{
			"tls": map[string]interface{}{
				"mode": "SIMPLE", // External APIs typically use simple TLS
			},
			"connectionPool": map[string]interface{}{
				"tcp": map[string]interface{}{
					"maxConnections": 100,
				},
			},
		},
	}

	if err := r.applyUnstructured(ctx, llmSvc, dr); err != nil {
		return fmt.Errorf("apply DestinationRule: %w", err)
	}

	return nil
}

func (r *ToolSurfaceReconciler) reconcilePeerAuthentication(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	pa := &unstructured.Unstructured{}
	pa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "security.istio.io",
		Kind:    "PeerAuthentication",
		Version: "v1beta1",
	})
	pa.SetName(llmSvc.Name)
	pa.SetNamespace(llmSvc.Namespace)
	pa.SetLabels(commonLabels(llmSvc))

	pa.Object["spec"] = map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app.kubernetes.io/instance": llmSvc.Name,
			},
		},
		"mtls": map[string]interface{}{
			"mode": "STRICT",
		},
	}

	return r.applyUnstructured(ctx, llmSvc, pa)
}

func (r *ToolSurfaceReconciler) reconcileIstioSidecar(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, fqdns []string) error {
	sc := &unstructured.Unstructured{}
	sc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Kind:    "Sidecar",
		Version: "v1beta1",
	})
	sc.SetName(llmSvc.Name)
	sc.SetNamespace(llmSvc.Namespace)
	sc.SetLabels(commonLabels(llmSvc))

	// The sidecar strictly limits the outbound visibility to the required namespaces and FQDNs.
	egressHosts := []interface{}{
		"./*",               // Local namespace for internal communication
		"istio-system/*",    // Istio control plane (required for sidecar operation)
		"knative-serving/*", // KServe internals if applicable
	}
	for _, f := range fqdns {
		egressHosts = append(egressHosts, "*/"+f)
	}

	sc.Object["spec"] = map[string]interface{}{
		"workloadSelector": map[string]interface{}{
			"labels": map[string]interface{}{
				"app.kubernetes.io/instance": llmSvc.Name,
			},
		},
		"egress": []interface{}{
			map[string]interface{}{
				"hosts": egressHosts,
			},
		},
	}

	return r.applyUnstructured(ctx, llmSvc, sc)
}

func (r *ToolSurfaceReconciler) applyUnstructured(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, obj *unstructured.Unstructured) error {
	if err := controllerutil.SetControllerReference(llmSvc, obj, r.Scheme); err != nil {
		return err
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return r.Create(ctx, obj)
		}
		return err
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, obj)
}

func commonLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}
}
