/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

const (
	rerankerFinalizer     = "serving.ckodex.com/reranker-finalizer"
	rerankerContainerName = "reranker"
)

// RerankerInferenceServiceReconciler reconciles RerankerInferenceService objects.
// It owns: Deployment, Service.
type RerankerInferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// AirGappedMode, when true, rewrites the vLLM image to LocalRegistry so pods
	// don't attempt to pull from the public internet.
	AirGappedMode bool
	LocalRegistry string
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=rerankerinferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=rerankerinferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=rerankerinferenceservices/finalizers,verbs=update

// Reconcile implements the main reconcile loop.
func (r *RerankerInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var svc servingv1alpha2.RerankerInferenceService
	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch RerankerInferenceService: %w", err)
	}

	// Handle deletion.
	if svc.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&svc, rerankerFinalizer) {
			controllerutil.RemoveFinalizer(&svc, rerankerFinalizer)
			if err := r.Update(ctx, &svc); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove reranker finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Register finalizer.
	if !controllerutil.ContainsFinalizer(&svc, rerankerFinalizer) {
		controllerutil.AddFinalizer(&svc, rerankerFinalizer)
		if err := r.Update(ctx, &svc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add reranker finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Apply WellKnown defaults before building resources.
	if preset := GetRerankerWellKnownConfig(svc.Spec.Model.URI); preset != nil {
		if svc.Spec.Resources == nil && preset.Resources != nil {
			svc.Spec.Resources = preset.Resources
		}
		if svc.Spec.MaxCandidates == 0 && preset.MaxCandidates > 0 {
			svc.Spec.MaxCandidates = preset.MaxCandidates
		}
	}

	// Reconcile Deployment.
	if err := r.reconcileDeployment(ctx, &svc); err != nil {
		_ = r.setCondition(ctx, &svc, servingv1alpha2.RerankerConditionDeploymentReady,
			metav1.ConditionFalse, "ReconcileError", err.Error())
		return ctrl.Result{}, fmt.Errorf("reconcile Reranker Deployment: %w", err)
	}

	// Reconcile Service.
	if err := r.reconcileService(ctx, &svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile Reranker Service: %w", err)
	}

	// Sync status.
	if err := r.syncStatus(ctx, &svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("sync Reranker status: %w", err)
	}

	logger.Info("RerankerInferenceService reconciled", "name", svc.Name)
	return ctrl.Result{}, nil
}

func (r *RerankerInferenceServiceReconciler) reconcileDeployment(
	ctx context.Context,
	svc *servingv1alpha2.RerankerInferenceService,
) error {
	desired := r.buildDeployment(svc)
	if err := controllerutil.SetControllerReference(svc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner ref on Deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get Deployment: %w", err)
	}

	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	return r.Update(ctx, existing)
}

func (r *RerankerInferenceServiceReconciler) reconcileService(
	ctx context.Context,
	svc *servingv1alpha2.RerankerInferenceService,
) error {
	desired := r.buildService(svc)
	if err := controllerutil.SetControllerReference(svc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner ref on Service: %w", err)
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get Service: %w", err)
	}
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

func (r *RerankerInferenceServiceReconciler) syncStatus(
	ctx context.Context,
	svc *servingv1alpha2.RerankerInferenceService,
) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: svc.Namespace, Name: svc.Name}, dep); err != nil {
		return fmt.Errorf("get Deployment for status: %w", err)
	}

	patch := client.MergeFrom(svc.DeepCopy())
	svc.Status.Replicas = dep.Status.ReadyReplicas
	svc.Status.ObservedGeneration = svc.Generation
	svc.Status.Endpoint = fmt.Sprintf(
		"http://%s.%s.svc.cluster.local/rerank",
		svc.Name, svc.Namespace,
	)

	ready := dep.Status.ReadyReplicas > 0
	condStatus := metav1.ConditionFalse
	condReason := "Unavailable"
	if ready {
		condStatus = metav1.ConditionTrue
		condReason = "Available"
	}
	msg := fmt.Sprintf("%d/%d pods ready", dep.Status.ReadyReplicas, dep.Status.Replicas)

	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.RerankerConditionDeploymentReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: svc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            msg,
	})
	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.RerankerConditionReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: svc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "Reranker service is ready to score document pairs",
	})
	return r.Status().Patch(ctx, svc, patch)
}

func (r *RerankerInferenceServiceReconciler) setCondition(
	ctx context.Context,
	svc *servingv1alpha2.RerankerInferenceService,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	patch := client.MergeFrom(svc.DeepCopy())
	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: svc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
	return r.Status().Patch(ctx, svc, patch)
}

// --- builders ---

func (r *RerankerInferenceServiceReconciler) buildDeployment(
	svc *servingv1alpha2.RerankerInferenceService,
) *appsv1.Deployment {
	labels := rerankerLabels(svc.Name)
	replicas := ptr.To(int32(1))
	if svc.Spec.Replicas != nil {
		replicas = svc.Spec.Replicas
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{r.buildContainer(svc)},
				},
			},
		},
	}
}

func (r *RerankerInferenceServiceReconciler) buildContainer(
	svc *servingv1alpha2.RerankerInferenceService,
) corev1.Container {
	modelID := rerankerModelID(svc.Spec.Model.URI)
	port := int32(servingv1alpha2.RerankerServerPort)

	args := []string{
		"--task", "score",
		"--model", modelID,
		"--host", "0.0.0.0",
		"--port", fmt.Sprintf("%d", port),
	}

	// Weight quantization (vLLM v0.24.0); GGUF is not applicable for rerankers.
	if q := svc.Spec.Quantization; q != nil && q.Method != "gguf" {
		args = append(args, "--quantization", q.Method)
	}

	res := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(api.DefaultVLLMCPURequest),
			corev1.ResourceMemory: resource.MustParse(api.DefaultVLLMMemoryRequest),
		},
	}
	if svc.Spec.Resources != nil {
		res = *svc.Spec.Resources
	}

	env := []corev1.EnvVar{
		{
			Name: "HF_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "hf-credentials"},
					Key:                  "HF_TOKEN",
					Optional:             ptr.To(true),
				},
			},
		},
	}

	image := api.VLLMImage
	if r.AirGappedMode && r.LocalRegistry != "" {
		image = rewriteImageRegistry(r.LocalRegistry, image)
	}
	return corev1.Container{
		Name:      rerankerContainerName,
		Image:     image,
		Args:      args,
		Resources: res,
		Ports:     []corev1.ContainerPort{{Name: "http", ContainerPort: port, Protocol: corev1.ProtocolTCP}},
		Env:       env,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       15,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			FailureThreshold:    6,
		},
	}
}

func (r *RerankerInferenceServiceReconciler) buildService(
	svc *servingv1alpha2.RerankerInferenceService,
) *corev1.Service {
	labels := rerankerLabels(svc.Name)
	port := int32(servingv1alpha2.RerankerServerPort)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// rerankerLabels returns the standard label set for reranker workloads.
func rerankerLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "rerankerinferenceservice",
		"app.kubernetes.io/instance":  name,
		"app.kubernetes.io/component": "reranker",
		"app.kubernetes.io/part-of":   "ckodex-llm-operator",
	}
}

// rewriteImageRegistry prepends localRegistry to image, replacing the original registry.
// Mirrors the logic in deployment/builder.go Builder.rewriteImage.
func rewriteImageRegistry(localRegistry, image string) string {
	parts := strings.Split(image, "/")
	return fmt.Sprintf("%s/%s", localRegistry, strings.Join(parts, "/"))
}

// rerankerModelID strips the hf:// URI scheme prefix to get the raw HuggingFace model ID.
func rerankerModelID(uri string) string {
	id, ok := strings.CutPrefix(uri, "hf://")
	if ok {
		return id
	}
	return uri
}

// SetupWithManager registers the controller with the manager.
func (r *RerankerInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.RerankerInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
