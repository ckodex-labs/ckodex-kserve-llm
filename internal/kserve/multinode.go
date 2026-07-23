/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package kserve

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	DefaultMultiNodeRuntime = "kserve-huggingfaceserver-multinode"

	deploymentModeAnnotation = "serving.kserve.io/deploymentMode"
	autoscalerAnnotation     = "serving.kserve.io/autoscalerClass"
)

var inferenceServiceGVK = schema.GroupVersionKind{
	Group: "serving.kserve.io", Version: "v1beta1", Kind: "InferenceService",
}

// Reconciler manages the upstream KServe InferenceService used for multi-node serving.
type Reconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	RuntimeName string
}

// RequiresMultiNode reports whether the service needs KServe's workerSpec path.
// Tensor parallelism alone remains a valid single-node vLLM deployment.
func RequiresMultiNode(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	if llmSvc.Spec.Worker != nil {
		return true
	}
	return llmSvc.Spec.Parallelism != nil &&
		llmSvc.Spec.Parallelism.Pipeline != nil &&
		*llmSvc.Spec.Parallelism.Pipeline > 1
}

// PredictorServiceName is the stable Service created by KServe Standard mode.
func PredictorServiceName(llmSvc *servingv1alpha2.LLMInferenceService) string {
	return llmSvc.Name + "-predictor"
}

// NewInferenceService returns an unstructured object with the KServe GVK set.
func NewInferenceService() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(inferenceServiceGVK)
	return obj
}

// Reconcile creates or updates the upstream KServe InferenceService.
func (r *Reconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	desired, err := r.Build(llmSvc)
	if err != nil {
		return err
	}

	existing := NewInferenceService()
	existing.SetName(llmSvc.Name)
	existing.SetNamespace(llmSvc.Namespace)
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		existing.Object["spec"] = desired.Object["spec"]

		labels := existing.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for key, value := range desired.GetLabels() {
			labels[key] = value
		}
		existing.SetLabels(labels)

		annotations := existing.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		for key, value := range desired.GetAnnotations() {
			annotations[key] = value
		}
		existing.SetAnnotations(annotations)
		if err := controllerutil.SetControllerReference(llmSvc, existing, r.Scheme); err != nil {
			return fmt.Errorf("set KServe InferenceService owner reference: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("create or update KServe InferenceService: %w", err)
	}
	return r.cleanupLegacyResources(ctx, llmSvc)
}

// Build maps the CKodex API to KServe v0.19's workerSpec contract.
func (r *Reconciler) Build(llmSvc *servingv1alpha2.LLMInferenceService) (*unstructured.Unstructured, error) {
	if err := validateMultiNode(llmSvc); err != nil {
		return nil, err
	}

	runtimeName := r.RuntimeName
	if runtimeName == "" {
		runtimeName = DefaultMultiNodeRuntime
	}
	tensor, pipeline := parallelismSizes(llmSvc.Spec.Parallelism)

	labels := workloadLabels(llmSvc)
	for key, value := range llmSvc.Spec.Template.Labels {
		labels[key] = value
	}
	if llmSvc.Spec.Worker != nil {
		for key, value := range llmSvc.Spec.Worker.Template.Labels {
			labels[key] = value
		}
	}
	annotations := copyStringMap(llmSvc.Spec.Template.Annotations)
	if llmSvc.Spec.Worker != nil {
		for key, value := range llmSvc.Spec.Worker.Template.Annotations {
			annotations[key] = value
		}
	}
	predictor := map[string]interface{}{
		"minReplicas": int64(1),
		"maxReplicas": int64(1),
		"labels":      stringMapToInterface(labels),
		"annotations": stringMapToInterface(annotations),
		"model": map[string]interface{}{
			"modelFormat": map[string]interface{}{"name": "huggingface"},
			"runtime":     runtimeName,
			"storageUri":  llmSvc.Spec.Model.URI,
		},
		"workerSpec": map[string]interface{}{
			"pipelineParallelSize": int64(pipeline),
			"tensorParallelSize":   int64(tensor),
		},
	}

	if err := applyHeadOverrides(predictor, llmSvc); err != nil {
		return nil, err
	}
	if err := applyWorkerOverrides(predictor["workerSpec"].(map[string]interface{}), llmSvc); err != nil {
		return nil, err
	}

	obj := NewInferenceService()
	obj.Object = map[string]interface{}{
		"apiVersion": "serving.kserve.io/v1beta1",
		"kind":       "InferenceService",
		"metadata": map[string]interface{}{
			"name":      llmSvc.Name,
			"namespace": llmSvc.Namespace,
			"labels":    stringMapToInterface(labels),
			"annotations": map[string]interface{}{
				deploymentModeAnnotation: "Standard",
				autoscalerAnnotation:     "none",
			},
		},
		"spec": map[string]interface{}{"predictor": predictor},
	}
	obj.SetGroupVersionKind(inferenceServiceGVK)
	return obj, nil
}

func validateMultiNode(llmSvc *servingv1alpha2.LLMInferenceService) error {
	if !RequiresMultiNode(llmSvc) {
		return fmt.Errorf("KServe multi-node requires spec.worker or pipeline parallelism greater than one")
	}
	if !strings.HasPrefix(llmSvc.Spec.Model.URI, "pvc://") &&
		!strings.HasPrefix(llmSvc.Spec.Model.URI, "oci://") {
		return fmt.Errorf("KServe v0.19 multi-node storage URI must use pvc:// or oci://, got %q", llmSvc.Spec.Model.URI)
	}
	if llmSvc.Spec.Replicas != nil && *llmSvc.Spec.Replicas != 1 {
		return fmt.Errorf("KServe v0.19 multi-node requires exactly one head replica")
	}
	if llmSvc.Spec.Scaling != nil {
		return fmt.Errorf("KServe v0.19 multi-node requires autoscaling to be disabled")
	}
	if llmSvc.Spec.Canary != nil {
		return fmt.Errorf("KServe v0.19 multi-node does not support CKodex canary routing")
	}
	if llmSvc.Spec.Prefill != nil {
		return fmt.Errorf("KServe v0.19 workerSpec cannot represent CKodex disaggregated prefill")
	}
	if storage := llmSvc.Spec.Model.Storage; storage != nil {
		if storage.SecretRef != nil || storage.StorageContainerRef != "" {
			return fmt.Errorf("KServe v0.19 multi-node storage credentials must use model.storage.serviceAccountName")
		}
	}
	if p := llmSvc.Spec.Parallelism; p != nil {
		if p.Data != nil && *p.Data > 1 {
			return fmt.Errorf("KServe v0.19 workerSpec cannot represent data parallelism")
		}
		if p.DataLocal != nil && *p.DataLocal > 1 {
			return fmt.Errorf("KServe v0.19 workerSpec cannot represent data-local parallelism")
		}
		if p.Expert || p.EPLBEnabled {
			return fmt.Errorf("KServe v0.19 workerSpec cannot represent expert parallelism or EPLB")
		}
	}
	return nil
}

func parallelismSizes(p *servingv1alpha2.ParallelismSpec) (int32, int32) {
	tensor, pipeline := int32(1), int32(1)
	if p == nil {
		return tensor, pipeline
	}
	if p.Tensor != nil {
		tensor = *p.Tensor
	}
	if p.Pipeline != nil {
		pipeline = *p.Pipeline
	}
	return tensor, pipeline
}

func applyHeadOverrides(predictor map[string]interface{}, llmSvc *servingv1alpha2.LLMInferenceService) error {
	podSpec, err := podOverrides(llmSvc.Spec.Template.Spec, false)
	if err != nil {
		return fmt.Errorf("head template: %w", err)
	}
	for key, value := range podSpec {
		predictor[key] = value
	}
	if len(llmSvc.Spec.Template.Spec.Containers) > 0 {
		container, err := containerOverrides(llmSvc.Spec.Template.Spec.Containers[0], "")
		if err != nil {
			return fmt.Errorf("convert head container overrides: %w", err)
		}
		for key, value := range container {
			predictor["model"].(map[string]interface{})[key] = value
		}
	}
	if storage := llmSvc.Spec.Model.Storage; storage != nil && storage.ServiceAccountName != "" {
		predictor["serviceAccountName"] = storage.ServiceAccountName
	}
	return nil
}

func applyWorkerOverrides(worker map[string]interface{}, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Worker == nil {
		return nil
	}
	podSpec, err := podOverrides(llmSvc.Spec.Worker.Template.Spec, true)
	if err != nil {
		return fmt.Errorf("worker template: %w", err)
	}
	for key, value := range podSpec {
		worker[key] = value
	}
	return nil
}

func podOverrides(spec corev1.PodSpec, includeContainer bool) (map[string]interface{}, error) {
	if len(spec.Containers) > 1 {
		return nil, fmt.Errorf("at most one container override is allowed")
	}
	if len(spec.Containers) == 1 {
		c := spec.Containers[0]
		if c.Image != "" || len(c.Command) > 0 || len(c.Args) > 0 {
			return nil, fmt.Errorf("container image, command, and args are owned by the KServe multi-node runtime")
		}
	}

	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return nil, fmt.Errorf("convert pod template: %w", err)
	}
	delete(raw, "containers")
	if includeContainer && len(spec.Containers) == 1 {
		container, err := containerOverrides(spec.Containers[0], "worker-container")
		if err != nil {
			return nil, fmt.Errorf("convert worker container overrides: %w", err)
		}
		raw["containers"] = []interface{}{container}
	}
	return raw, nil
}

func containerOverrides(container corev1.Container, name string) (map[string]interface{}, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&container)
	if err != nil {
		return nil, err
	}
	delete(raw, "name")
	delete(raw, "image")
	delete(raw, "command")
	delete(raw, "args")
	if name != "" {
		raw["name"] = name
	}
	return raw, nil
}

func workloadLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}
}

func stringMapToInterface(values map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func copyStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (r *Reconciler) cleanupLegacyResources(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	key := types.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace}
	objects := []client.Object{
		&appsv1.Deployment{},
		&corev1.Service{},
		&policyv1.PodDisruptionBudget{},
		&autoscalingv2.HorizontalPodAutoscaler{},
	}
	for _, obj := range objects {
		if err := r.Get(ctx, key, obj); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("get legacy %T: %w", obj, err)
		}
		if !metav1.IsControlledBy(obj, llmSvc) {
			continue
		}
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete legacy %T: %w", obj, err)
		}
	}

	lws := &unstructured.Unstructured{}
	lws.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet",
	})
	lwsKey := types.NamespacedName{Name: llmSvc.Name + "-lws", Namespace: llmSvc.Namespace}
	if err := r.Get(ctx, lwsKey, lws); err == nil && metav1.IsControlledBy(lws, llmSvc) {
		if err := r.Delete(ctx, lws); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete legacy LeaderWorkerSet: %w", err)
		}
	} else if err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("get legacy LeaderWorkerSet: %w", err)
	}
	return nil
}
