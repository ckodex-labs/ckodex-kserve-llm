/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"maps"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	controllerreconciler "github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
)

const (
	// multimodalFinalizer is the finalizer the Multimodal controller registers on each CR.
	multimodalFinalizer = "serving.ckodex.com/multimodal-finalizer"

	// multimodalContainerName is the name of the primary serving container.
	multimodalContainerName = "multimodal-server"
)

// MultimodalInferenceServiceReconciler reconciles MultimodalInferenceService objects.
// It owns: Deployment, Service.
// It does NOT own: Gateway, HTTPRoute — external exposure is left to the user.
//
// VLMs (vision-language task) typically require GPU resources.
// Image generation models may require large amounts of GPU memory.
// Resource requirements are configured via spec.template.
type MultimodalInferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=multimodalinferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=multimodalinferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=multimodalinferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the main reconcile loop.
func (r *MultimodalInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var mmSvc servingv1alpha2.MultimodalInferenceService
	if err := r.Get(ctx, req.NamespacedName, &mmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch MultimodalInferenceService: %w", err)
	}

	// Capture original object for diffing and patching at the end
	mmSvcBeforePatch := mmSvc.DeepCopy()

	// Handle deletion.
	if mmSvc.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&mmSvc, multimodalFinalizer) {
			controllerutil.RemoveFinalizer(&mmSvc, multimodalFinalizer)
			if err := r.Update(ctx, &mmSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove multimodal finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Register finalizer.
	if !controllerutil.ContainsFinalizer(&mmSvc, multimodalFinalizer) {
		controllerutil.AddFinalizer(&mmSvc, multimodalFinalizer)
		if err := r.Update(ctx, &mmSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add multimodal finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// 1. Validate Model URI (strictly hf:// for now)
	if !strings.HasPrefix(mmSvc.Spec.Model.URI, "hf://") {
		if err := r.setMultimodalCondition(ctx, &mmSvc, servingv1alpha2.MultimodalConditionReady,
			metav1.ConditionFalse, "InvalidModelURI",
			fmt.Sprintf("Model URI %q is not supported. Use hf://repo/model", mmSvc.Spec.Model.URI)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch invalid URI condition: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// 2. Reconcile Deployment.
	if err := r.reconcileMultimodalDeployment(ctx, &mmSvc); err != nil {
		_ = r.setMultimodalCondition(ctx, &mmSvc, servingv1alpha2.MultimodalConditionDeploymentReady,
			metav1.ConditionFalse, "ReconcileError", err.Error())
		return ctrl.Result{}, fmt.Errorf("reconcile Multimodal Deployment: %w", err)
	}

	// 3. Reconcile Service.
	if err := r.reconcileMultimodalService(ctx, &mmSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile Multimodal Service: %w", err)
	}

	// 4. Sync status.
	if err := r.syncMultimodalStatus(ctx, &mmSvc, mmSvcBeforePatch); err != nil {
		return ctrl.Result{}, fmt.Errorf("sync Multimodal status: %w", err)
	}

	logger.Info("MultimodalInferenceService reconciled",
		"name", mmSvc.Name,
		"task", mmSvc.Spec.Task,
		"runtime", mmSvc.Spec.Runtime,
	)
	return ctrl.Result{}, nil
}

func (r *MultimodalInferenceServiceReconciler) reconcileMultimodalDeployment(
	ctx context.Context,
	mmSvc *servingv1alpha2.MultimodalInferenceService,
) error {
	desired := r.buildMultimodalDeployment(mmSvc)
	if err := controllerutil.SetControllerReference(mmSvc, desired, r.Scheme); err != nil {
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

	if !controllerreconciler.SyncDeployment(ctx, existing, desired, replicaCount(desired.Spec.Replicas), false) {
		return nil
	}
	return r.Update(ctx, existing)
}

func (r *MultimodalInferenceServiceReconciler) reconcileMultimodalService(
	ctx context.Context,
	mmSvc *servingv1alpha2.MultimodalInferenceService,
) error {
	desired := r.buildMultimodalService(mmSvc)
	if err := controllerutil.SetControllerReference(mmSvc, desired, r.Scheme); err != nil {
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

func (r *MultimodalInferenceServiceReconciler) syncMultimodalStatus(
	ctx context.Context,
	mmSvc *servingv1alpha2.MultimodalInferenceService,
	mmSvcBeforePatch *servingv1alpha2.MultimodalInferenceService,
) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: mmSvc.Namespace,
		Name:      mmSvc.Name,
	}, dep); err != nil {
		if apierrors.IsNotFound(err) {
			mmSvc.Status.Replicas = 0
		} else {
			return fmt.Errorf("get Deployment for status sync: %w", err)
		}
	} else {
		mmSvc.Status.Replicas = dep.Status.ReadyReplicas
	}
	mmSvc.Status.ObservedGeneration = mmSvc.Generation
	mmSvc.Status.URL = multimodalEndpointURL(mmSvc)

	// Update conditions based on deployment readiness.
	depReady := dep.Status.ReadyReplicas > 0
	condStatus := metav1.ConditionFalse
	condReason := "Unavailable"
	if depReady {
		condStatus = metav1.ConditionTrue
		condReason = "Available"
	}

	meta.SetStatusCondition(&mmSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.MultimodalConditionDeploymentReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: mmSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("%d/%d pods ready", dep.Status.ReadyReplicas, dep.Status.Replicas),
	})
	meta.SetStatusCondition(&mmSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.MultimodalConditionReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: mmSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "Multimodal service is ready to serve requests",
	})

	// Only patch status if it actually changed to avoid infinite reconciliation loops.
	if !equality.Semantic.DeepEqual(&mmSvcBeforePatch.Status, &mmSvc.Status) {
		err := r.Status().Patch(ctx, mmSvc, client.MergeFrom(mmSvcBeforePatch))
		if err != nil {
			if apierrors.IsConflict(err) {
				// Conflict is benign during status updates; we'll retry on next reconciliation
				return nil
			}
			return err
		}
	}
	return nil
}

func (r *MultimodalInferenceServiceReconciler) setMultimodalCondition(
	ctx context.Context,
	mmSvc *servingv1alpha2.MultimodalInferenceService,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	patch := client.MergeFrom(mmSvc.DeepCopy())
	meta.SetStatusCondition(&mmSvc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: mmSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
	return r.Status().Patch(ctx, mmSvc, patch)
}

// --- builders ---

// buildMultimodalDeployment constructs the desired Deployment.
func (r *MultimodalInferenceServiceReconciler) buildMultimodalDeployment(
	mmSvc *servingv1alpha2.MultimodalInferenceService,
) *appsv1.Deployment {
	labels := multimodalLabels(mmSvc.Name)
	replicas := ptr.To(int32(1))
	if mmSvc.Spec.Replicas != nil {
		replicas = mmSvc.Spec.Replicas
	}

	podTemplate := mmSvc.Spec.Template.DeepCopy()
	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}
	maps.Copy(podTemplate.Labels, labels)

	serverContainer := r.buildMultimodalContainer(mmSvc)
	podTemplate.Spec.Containers = append([]corev1.Container{serverContainer}, podTemplate.Spec.Containers...)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmSvc.Name,
			Namespace: mmSvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: *podTemplate,
		},
	}
}

// buildMultimodalContainer constructs the primary vLLM container with multimodal flags.
//
// FACT: vLLM serves VLMs through the same entrypoint as text models.
// The --limit-mm-per-prompt flag controls how many images are accepted per request.
// Model architecture detection is automatic — no explicit flag needed.
func (r *MultimodalInferenceServiceReconciler) buildMultimodalContainer(
	mmSvc *servingv1alpha2.MultimodalInferenceService,
) corev1.Container {
	img := mmSvc.Spec.RuntimeImage
	if img == "" {
		img = servingv1alpha2.DefaultMultimodalRuntimeImage(mmSvc.Spec.Runtime)
	}
	if img == "" {
		img = servingv1alpha2.DefaultMultimodalRuntimeImage(servingv1alpha2.MultimodalRuntimeVLLM)
	}

	modelID, _ := strings.CutPrefix(mmSvc.Spec.Model.URI, "hf://")

	maxImages := int32(1)
	if mmSvc.Spec.MaxImagesPerPrompt != nil {
		maxImages = *mmSvc.Spec.MaxImagesPerPrompt
	}

	args := []string{
		"--model", modelID,
		"--port", fmt.Sprintf("%d", servingv1alpha2.MultimodalServerPort),
		"--host", "0.0.0.0",
	}

	// Handle task-specific configurations.
	switch mmSvc.Spec.Task {
	case servingv1alpha2.MultimodalTaskVisionLanguage:
		args = append(args, "--limit-mm-per-prompt", fmt.Sprintf("image=%d", maxImages))
	case servingv1alpha2.MultimodalTaskTextToSpeech:
		// LiquidAI Audio models often need trust-remote-code for specialized kernels.
		if strings.Contains(strings.ToLower(modelID), "liquidai") {
			args = append(args, "--trust-remote-code")
		}
		// vLLM TTS experimental support requires specific flags.
		args = append(args, "--enforce-eager")
	}

	// Weight quantization (vLLM v0.24.0); GGUF is unsupported for VLMs.
	if q := mmSvc.Spec.Quantization; q != nil && q.Method != "gguf" {
		args = append(args, "--quantization", q.Method)
	}

	// Inject HF_TOKEN from canonical secret (optional — gated VLMs require it).
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

	port := int32(servingv1alpha2.MultimodalServerPort)
	return corev1.Container{
		Name:  multimodalContainerName,
		Image: img,
		Args:  args,
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: port, Protocol: corev1.ProtocolTCP},
		},
		Env: env,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       20,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt32(port),
				},
			},
			// VLMs have large weights — generous initial delay for model download.
			InitialDelaySeconds: 60,
			PeriodSeconds:       15,
			FailureThreshold:    12,
		},
	}
}

// buildMultimodalService constructs the desired ClusterIP Service.
func (r *MultimodalInferenceServiceReconciler) buildMultimodalService(
	mmSvc *servingv1alpha2.MultimodalInferenceService,
) *corev1.Service {
	labels := multimodalLabels(mmSvc.Name)
	port := int32(servingv1alpha2.MultimodalServerPort)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmSvc.Name,
			Namespace: mmSvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// multimodalEndpointURL returns the cluster-local API URL for the given task.
func multimodalEndpointURL(mmSvc *servingv1alpha2.MultimodalInferenceService) string {
	base := fmt.Sprintf("http://%s.%s.svc.cluster.local", mmSvc.Name, mmSvc.Namespace)
	switch mmSvc.Spec.Task {
	case servingv1alpha2.MultimodalTaskImageGeneration:
		return base + "/v1/images/generations"
	case servingv1alpha2.MultimodalTaskTextToSpeech:
		return base + "/v1/audio/speech"
	default: // vision-language
		return base + "/v1/chat/completions"
	}
}

// multimodalLabels returns the standard label set for multimodal workloads.
func multimodalLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "multimodalinferenceservice",
		"app.kubernetes.io/instance":  name,
		"app.kubernetes.io/component": "multimodal-server",
		"app.kubernetes.io/part-of":   "ckodex-llm-operator",
	}
}

// SetupWithManager registers the controller with the manager.
func (r *MultimodalInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.MultimodalInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
