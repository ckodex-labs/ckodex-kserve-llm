/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package autoscaler implements WVA, KEDA, and HPA autoscaling reconcilers.
package autoscaler

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// Reconciler manages autoscaling resources (HPA/KEDA/WVA).
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates the appropriate autoscaler based on spec.
func (r *Reconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Scaling == nil {
		return nil
	}

	logger := log.FromContext(ctx).WithValues("component", "autoscaler")

	// Priority: WVA > KEDA > HPA
	if llmSvc.Spec.Scaling.WVA != nil {
		if err := r.reconcileWVA(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile WVA: %w", err)
		}
		logger.Info("WVA autoscaling reconciled")
		return nil
	}

	if llmSvc.Spec.Scaling.KEDA != nil {
		if err := r.reconcileKEDA(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile KEDA: %w", err)
		}
		logger.Info("KEDA autoscaling reconciled")
		return nil
	}

	// Fallback: standard HPA
	return r.reconcileHPA(ctx, llmSvc)
}

// reconcileKEDA creates/updates a KEDA ScaledObject for scale-to-zero.
func (r *Reconciler) reconcileKEDA(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	name := llmSvc.Name + "-scaledobject"
	minReplicas := int64(0) // KEDA enables scale-to-zero
	maxReplicas := int64(10)
	if llmSvc.Spec.Scaling.MinReplicas != nil {
		minReplicas = int64(*llmSvc.Spec.Scaling.MinReplicas)
	}
	if llmSvc.Spec.Scaling.MaxReplicas != nil {
		maxReplicas = int64(*llmSvc.Spec.Scaling.MaxReplicas)
	}

	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
					"app.kubernetes.io/instance":   llmSvc.Name,
				},
			},
			"spec": map[string]interface{}{
				"scaleTargetRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       llmSvc.Name,
				},
				"pollingInterval": int64(10),  // Faster polling for real-time inference
				"cooldownPeriod":  int64(120), // Quicker scale-down to save costs
				"minReplicaCount": minReplicas,
				"maxReplicaCount": maxReplicas,
				"triggers": []interface{}{
					map[string]interface{}{
						"type": "prometheus",
						"metadata": map[string]interface{}{
							"serverAddress": "http://prometheus.monitoring:9090",
							"metricName":    "vllm_requests_waiting",
							"query":         fmt.Sprintf(`sum(vllm_requests_waiting{model_name="%s"})`, llmSvc.Spec.Model.Name),
							"threshold":     "5", // Scale up if queue > 5
						},
					},
					map[string]interface{}{
						"type": "prometheus",
						"metadata": map[string]interface{}{
							"serverAddress": "http://prometheus.monitoring:9090",
							"metricName":    "vllm_avg_ttft_ms",
							"query":         fmt.Sprintf(`avg_over_time(vllm_ttft_ms{model_name="%s"}[1m])`, llmSvc.Spec.Model.Name),
							"threshold":     "500", // Scale up if TTFT > 500ms
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
			return r.Create(ctx, desired)
		}
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// reconcileWVA creates/updates a WorkloadVariantAutoscaler CR.
func (r *Reconciler) reconcileWVA(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	name := llmSvc.Name + "-wva"
	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "autoscaling.x-k8s.io/v1alpha1",
			"kind":       "WorkloadVariantAutoscaler",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
					"app.kubernetes.io/instance":   llmSvc.Name,
				},
			},
			"spec": map[string]interface{}{
				"targetRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       llmSvc.Name,
				},
				"variants": []interface{}{
					map[string]interface{}{
						"name": "high-throughput",
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"nvidia.com/gpu": "1",
							},
						},
						"minReplicas": int64(1),
						"maxReplicas": int64(4),
					},
					map[string]interface{}{
						"name": "cost-optimized",
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"nvidia.com/gpu": "1",
							},
						},
						"minReplicas": int64(0),
						"maxReplicas": int64(2),
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
			return r.Create(ctx, desired)
		}
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// reconcileHPA creates/updates a standard HPA.
func (r *Reconciler) reconcileHPA(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Scaling.MinReplicas == nil && llmSvc.Spec.Scaling.MaxReplicas == nil {
		return nil
	}

	name := llmSvc.Name + "-hpa"
	minReplicas := int32(1)
	maxReplicas := int32(10)
	if llmSvc.Spec.Scaling.MinReplicas != nil {
		minReplicas = *llmSvc.Spec.Scaling.MinReplicas
	}
	if llmSvc.Spec.Scaling.MaxReplicas != nil {
		maxReplicas = *llmSvc.Spec.Scaling.MaxReplicas
	}

	avgUtil := int32(70)
	desired := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: llmSvc.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				"app.kubernetes.io/instance":   llmSvc.Name,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       llmSvc.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &avgUtil,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing autoscalingv2.HorizontalPodAutoscaler
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}
	existing.Spec = desired.Spec
	return r.Update(ctx, &existing)
}
