/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

var inferencePoolGVK = schema.GroupVersionKind{
	Group: "inference.networking.k8s.io", Version: "v1", Kind: "InferencePool",
}

// InferencePoolManager reconciles the GA Gateway API Inference Extension pool.
type InferencePoolManager struct {
	client.Client
	Scheme *runtime.Scheme
}

func (m *InferencePoolManager) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Router.Scheduler == nil {
		return fmt.Errorf("scheduler is not configured")
	}
	selector := llmSvc.Spec.Router.Scheduler.Pool.Selector
	if len(selector) == 0 {
		selector = map[string]string{
			"app.kubernetes.io/name":     "llminferenceservice",
			"app.kubernetes.io/instance": llmSvc.Name,
		}
	}
	desired := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "inference.networking.k8s.io/v1",
		"kind":       "InferencePool",
		"metadata": map[string]interface{}{
			"name": llmSvc.Name, "namespace": llmSvc.Namespace,
			"labels": map[string]interface{}{"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator"},
		},
		"spec": map[string]interface{}{
			"selector":    map[string]interface{}{"matchLabels": stringMapToAny(selector)},
			"targetPorts": []interface{}{map[string]interface{}{"number": int64(8000)}},
			"endpointPickerRef": map[string]interface{}{
				"name":        llmSvc.Name + "-epp",
				"port":        map[string]interface{}{"number": int64(EPPPort)},
				"failureMode": "FailClose",
			},
		},
	}}
	desired.SetGroupVersionKind(inferencePoolGVK)
	if err := controllerutil.SetControllerReference(llmSvc, desired, m.Scheme); err != nil {
		return fmt.Errorf("set inferencepool owner: %w", err)
	}
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(inferencePoolGVK)
	err := m.Get(ctx, types.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return m.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get GA InferencePool (is the v1 CRD installed?): %w", err)
	}
	existing.Object["spec"] = desired.Object["spec"]
	existing.SetLabels(desired.GetLabels())
	return m.Update(ctx, existing)
}

func stringMapToAny(src map[string]string) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
