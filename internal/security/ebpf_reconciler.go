/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// GVK for Tetragon TracingPolicy
var TracingPolicyGVK = schema.GroupVersionKind{
	Group:   "isovalent.com",
	Version: "v1alpha1",
	Kind:    "TracingPolicy",
}

// EbpfReconciler manages Tetragon TracingPolicies for eBPF-based security.
type EbpfReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ReconcileEbpfPolicy creates or updates Tetragon TracingPolicies for the inference service.
// Two policies are created:
//   - <name>-security-policy : traces sys_execve (process execution audit)
//   - <name>-network-policy  : traces sys_connect + sys_accept (network flow audit)
func (r *EbpfReconciler) ReconcileEbpfPolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "ebpf")

	// 1. Check if Tetragon is available
	exists, err := r.isGVKAvailable()
	if err != nil {
		return fmt.Errorf("check Tetragon availability: %w", err)
	}
	if !exists {
		logger.Info("Tetragon isovalent.com/v1alpha1 TracingPolicy CRDs not found, skipping eBPF reconciliation")
		return nil
	}

	name := llmSvc.Name + "-security-policy"
	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "isovalent.com/v1alpha1",
			"kind":       "TracingPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
					"app.kubernetes.io/instance":   llmSvc.Name,
				},
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/instance": llmSvc.Name,
					},
				},
				"kprobes": []interface{}{
					map[string]interface{}{
						"call":    "sys_execve",
						"syscall": true,
						"args": []interface{}{
							map[string]interface{}{"index": int64(0), "type": "string"},
						},
						"selectors": []interface{}{
							map[string]interface{}{
								"matchActions": []interface{}{
									map[string]interface{}{"action": "Post"},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing unstructured.Unstructured
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("creating Tetragon TracingPolicy", "name", name)
			if err := r.Create(ctx, desired); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		desired.SetResourceVersion(existing.GetResourceVersion())
		logger.Info("updating Tetragon TracingPolicy", "name", name)
		if err := r.Update(ctx, desired); err != nil {
			return err
		}
	}

	if err := r.reconcileNetworkPolicy(ctx, llmSvc); err != nil {
		return err
	}
	return r.reconcileMemoryPolicy(ctx, llmSvc)
}

// isGVKAvailable checks if Tetragon TracingPolicy CRD is registered.
func (r *EbpfReconciler) isGVKAvailable() (bool, error) {
	if r.Client == nil {
		return false, nil
	}

	if mapper := r.RESTMapper(); mapper != nil {
		_, err := mapper.RESTMapping(TracingPolicyGVK.GroupKind(), TracingPolicyGVK.Version)
		if err == nil {
			return true, nil
		}
		if !meta.IsNoMatchError(err) {
			return false, err
		}
	}

	if r.Scheme != nil && r.Scheme.Recognizes(TracingPolicyGVK) {
		return true, nil
	}

	return false, nil
}

// reconcileNetworkPolicy creates a Tetragon TracingPolicy that traces network syscalls.
func (r *EbpfReconciler) reconcileNetworkPolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(context.Background()).WithValues("component", "ebpf")
	name := llmSvc.Name + "-network-policy"

	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "isovalent.com/v1alpha1",
			"kind":       "TracingPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
					"app.kubernetes.io/instance":   llmSvc.Name,
				},
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/instance": llmSvc.Name,
					},
				},
				"kprobes": []interface{}{
					map[string]interface{}{
						"call":    "sys_connect",
						"syscall": true,
						"args": []interface{}{
							map[string]interface{}{"index": int64(0), "type": "int"},
							map[string]interface{}{"index": int64(1), "type": "sockaddr"},
						},
						"selectors": []interface{}{
							map[string]interface{}{
								"matchActions": []interface{}{
									map[string]interface{}{"action": "Post"},
								},
							},
						},
					},
					map[string]interface{}{
						"call":    "sys_accept",
						"syscall": true,
						"args": []interface{}{
							map[string]interface{}{"index": int64(0), "type": "int"},
						},
						"selectors": []interface{}{
							map[string]interface{}{
								"matchActions": []interface{}{
									map[string]interface{}{"action": "Post"},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference on network policy: %w", err)
	}

	var existing unstructured.Unstructured
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("creating Tetragon network TracingPolicy", "name", name)
			return r.Create(ctx, desired)
		}
		return err
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("updating Tetragon network TracingPolicy", "name", name)
	return r.Update(ctx, desired)
}

// reconcileMemoryPolicy creates a Tetragon TracingPolicy that mitigates dynamic memory injection.
func (r *EbpfReconciler) reconcileMemoryPolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(context.Background()).WithValues("component", "ebpf")
	name := llmSvc.Name + "-memory-policy"

	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "isovalent.com/v1alpha1",
			"kind":       "TracingPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
					"app.kubernetes.io/instance":   llmSvc.Name,
				},
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/instance": llmSvc.Name,
					},
				},
				"kprobes": []interface{}{
					map[string]interface{}{
						"call":    "sys_ptrace",
						"syscall": true,
						"args": []interface{}{
							map[string]interface{}{"index": int64(0), "type": "int"},
						},
						"selectors": []interface{}{
							map[string]interface{}{
								"matchActions": []interface{}{
									map[string]interface{}{"action": "Sigkill"},
								},
							},
						},
					},
					map[string]interface{}{
						"call":    "sys_mprotect",
						"syscall": true,
						"args": []interface{}{
							map[string]interface{}{"index": int64(0), "type": "int"},
							map[string]interface{}{"index": int64(1), "type": "int"},
							map[string]interface{}{"index": int64(2), "type": "int"},
						},
						"selectors": []interface{}{
							map[string]interface{}{
								"matchActions": []interface{}{
									map[string]interface{}{"action": "Post"},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference on memory policy: %w", err)
	}

	var existing unstructured.Unstructured
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("creating Tetragon memory TracingPolicy", "name", name)
			return r.Create(ctx, desired)
		}
		return err
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("updating Tetragon memory TracingPolicy", "name", name)
	return r.Update(ctx, desired)
}
