/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const endpointPickerConfigAPIVersion = "llm-d.ai/v1alpha1"

// ConfigReconciler reconciles EndpointPickerConfig into a ConfigMap
// consumed by the EPP container.
type ConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates/updates the scheduler ConfigMap from EndpointPickerConfig.
func (r *ConfigReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "scheduler-config")

	configName := schedulerConfigName(llmSvc.Name)

	data, err := r.effectiveConfig(ctx, llmSvc)
	if err != nil {
		return err
	}

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
	original := existing.DeepCopy()
	if err := controllerutil.SetControllerReference(llmSvc, &existing, r.Scheme); err != nil {
		return fmt.Errorf("set existing configmap owner reference: %w", err)
	}
	existing.Labels = desired.Labels
	existing.Data = desired.Data
	if apiequality.Semantic.DeepEqual(&existing, original) {
		return nil
	}
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *ConfigReconciler) effectiveConfig(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) (map[string]string, error) {
	if llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Config == nil {
		return r.buildDefaultConfig(llmSvc), nil
	}
	config := llmSvc.Spec.Router.Scheduler.Config
	if config.Inline != nil {
		data, err := json.Marshal(map[string]interface{}{
			"apiVersion":         endpointPickerConfigAPIVersion,
			"kind":               "EndpointPickerConfig",
			"plugins":            config.Inline.Plugins,
			"schedulingProfiles": config.Inline.SchedulingProfiles,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal inline scheduler config: %w", err)
		}
		return map[string]string{"scheduler.yaml": string(data)}, nil
	}
	if config.Ref != nil {
		var source corev1.ConfigMap
		if err := r.Get(ctx, types.NamespacedName{Name: config.Ref.Name, Namespace: llmSvc.Namespace}, &source); err != nil {
			return nil, fmt.Errorf("get scheduler configmap %s: %w", config.Ref.Name, err)
		}
		key := config.Ref.Key
		if key == "" {
			key = "endpoint-picker-config.yaml"
		}
		value, ok := source.Data[key]
		if !ok {
			return nil, fmt.Errorf("scheduler configmap %s is missing key %s", config.Ref.Name, key)
		}
		return map[string]string{"scheduler.yaml": value}, nil
	}
	return r.buildDefaultConfig(llmSvc), nil
}

// buildDefaultConfig generates the default EPP scheduler plugin pipeline.
// Matches KServe v0.17: prefix-cache-scorer, queue-scorer, kv-cache-utilization-scorer, max-score-picker.
func (r *ConfigReconciler) buildDefaultConfig(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"scheduler.yaml": fmt.Sprintf(`# Auto-generated scheduler config for %s
apiVersion: %s
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
- type: kv-cache-utilization-scorer
- type: prefix-cache-scorer
- type: metrics-data-source
  parameters:
    scheme: "http"
    path: "/metrics"
    insecureSkipVerify: true
- type: core-metrics-extractor
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: kv-cache-utilization-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
`, llmSvc.Name, endpointPickerConfigAPIVersion),
	}
}
