/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ConfigReconciler reconciles EndpointPickerConfig into a ConfigMap
// consumed by the EPP container.
type ConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates/updates the scheduler ConfigMap from EndpointPickerConfig.
func (r *ConfigReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "scheduler-config")

	configName := llmSvc.Name + "-scheduler-config"

	// Build default scheduler config if no EndpointPickerConfig ref
	data := r.buildDefaultConfig(llmSvc)

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configName,
			Namespace: llmSvc.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				"serving.ckodex.com/role":      "scheduler-config",
			},
		},
		Data: data,
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: configName, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("creating scheduler ConfigMap", "name", configName)
			return r.Create(ctx, desired)
		}
		return err
	}
	existing.Data = desired.Data
	return r.Update(ctx, &existing)
}

// buildDefaultConfig generates the default EPP scheduler plugin pipeline.
// Matches KServe v0.17: prefix-cache-scorer, queue-scorer, kv-cache-utilization-scorer, max-score-picker.
func (r *ConfigReconciler) buildDefaultConfig(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"scheduler.yaml": fmt.Sprintf(`# Auto-generated scheduler config for %s
profiles:
  - name: default
    plugins:
      - pluginRef: prefix-cache-scorer
        weight: "2.0"
      - pluginRef: queue-scorer
        weight: "2.0"
      - pluginRef: kv-cache-utilization-scorer
        weight: "2.0"
      - pluginRef: max-score-picker
        weight: "1.0"
`, llmSvc.Name),
	}
}
