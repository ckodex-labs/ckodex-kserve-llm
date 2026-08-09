/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	operatorconfig "github.com/ckodex-labs/kserve-llm-operator/internal/config"
	controllerreconciler "github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
)

const (
	// asrFinalizer is the finalizer the ASR controller registers on each CR.
	asrFinalizer = "serving.ckodex.com/asr-finalizer"

	// asrServerPort is the port the ASR runtime container listens on.
	// Both faster-whisper-server and the transformers runtime use 8000.
	asrServerPort = 8000

	// asrContainerName is the name of the primary serving container in the Deployment.
	asrContainerName = "asr-server"
)

// ASRInferenceServiceReconciler reconciles ASRInferenceService objects.
// It owns: Deployment, Service.
// It does NOT own: Gateway, HTTPRoute — ASR services are cluster-internal by default.
// External exposure is intentionally left to the user (Ingress/Gateway over HTTPS
// because audio payloads must not be transmitted in cleartext in production).
type ASRInferenceServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Defaults operatorconfig.DefaultsConfig
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=asrinferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=asrinferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=asrinferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the main reconcile loop.
func (r *ASRInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var asrSvc servingv1alpha2.ASRInferenceService
	if err := r.Get(ctx, req.NamespacedName, &asrSvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch ASRInferenceService: %w", err)
	}
	// Capture original object for diffing and patching at the end
	asrSvcBeforePatch := asrSvc.DeepCopy()

	// Handle deletion.
	if asrSvc.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&asrSvc, asrFinalizer) {
			if err := r.cleanupASR(ctx, &asrSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("cleanup ASR resources: %w", err)
			}
			controllerutil.RemoveFinalizer(&asrSvc, asrFinalizer)
			if err := r.Update(ctx, &asrSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove ASR finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Register finalizer.
	if !controllerutil.ContainsFinalizer(&asrSvc, asrFinalizer) {
		controllerutil.AddFinalizer(&asrSvc, asrFinalizer)
		if err := r.Update(ctx, &asrSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add ASR finalizer: %w", err)
		}
	}

	// Validate custom runtimes require a user-supplied image.
	if (asrSvc.Spec.Runtime == servingv1alpha2.ASRRuntimeTransformers ||
		asrSvc.Spec.Runtime == servingv1alpha2.ASRRuntimeCustom) && asrSvc.Spec.RuntimeImage == "" {
		if err := r.setCondition(ctx, &asrSvc, asrSvcBeforePatch, servingv1alpha2.ASRConditionReady,
			metav1.ConditionFalse, "MissingRuntimeImage",
			fmt.Sprintf("runtime=%s requires spec.runtimeImage to be set", asrSvc.Spec.Runtime)); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("ASRInferenceService blocked: custom runtime but no runtimeImage", "name", asrSvc.Name, "runtime", asrSvc.Spec.Runtime)
		return ctrl.Result{}, nil
	}

	// Reconcile Deployment.
	if err := r.reconcileASRDeployment(ctx, &asrSvc); err != nil {
		_ = r.setCondition(ctx, &asrSvc, asrSvcBeforePatch, servingv1alpha2.ASRConditionDeploymentReady,
			metav1.ConditionFalse, "ReconcileError", err.Error())
		return ctrl.Result{}, fmt.Errorf("reconcile ASR Deployment: %w", err)
	}

	// Reconcile Service.
	if err := r.reconcileASRService(ctx, &asrSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile ASR Service: %w", err)
	}

	// Update status from the live Deployment.
	if err := r.syncASRStatus(ctx, &asrSvc, asrSvcBeforePatch); err != nil {
		return ctrl.Result{}, fmt.Errorf("sync ASR status: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileASRDeployment creates or updates the Deployment for the ASR service.
func (r *ASRInferenceServiceReconciler) reconcileASRDeployment(
	ctx context.Context,
	asrSvc *servingv1alpha2.ASRInferenceService,
) error {
	desired := r.buildASRDeployment(asrSvc)
	if err := controllerutil.SetControllerReference(asrSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner ref on Deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			r.Recorder.Eventf(asrSvc, corev1.EventTypeWarning, "DeploymentCreationFailed",
				"Failed to create ASR Deployment %s: %v", desired.Name, err)
			return err
		}
		r.Recorder.Eventf(asrSvc, corev1.EventTypeNormal, "DeploymentCreated",
			"Successfully created ASR Deployment %s", desired.Name)
		return nil
	}
	if err != nil {
		r.Recorder.Eventf(asrSvc, corev1.EventTypeWarning, "DeploymentLookupFailed",
			"Failed to look up ASR Deployment %s: %v", desired.Name, err)
		return fmt.Errorf("get Deployment: %w", err)
	}

	if !controllerreconciler.SyncDeployment(ctx, existing, desired, replicaCount(desired.Spec.Replicas), false) {
		return nil
	}
	return r.Update(ctx, existing)
}

func replicaCount(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

// reconcileASRService creates or updates the ClusterIP Service.
func (r *ASRInferenceServiceReconciler) reconcileASRService(
	ctx context.Context,
	asrSvc *servingv1alpha2.ASRInferenceService,
) error {
	desired := r.buildASRService(asrSvc)
	if err := controllerutil.SetControllerReference(asrSvc, desired, r.Scheme); err != nil {
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
	// Service ClusterIP is immutable — only update selector and ports.
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

// syncASRStatus reads the live Deployment and updates the CR status.
func (r *ASRInferenceServiceReconciler) syncASRStatus(
	ctx context.Context,
	asrSvc *servingv1alpha2.ASRInferenceService,
	asrSvcBeforePatch *servingv1alpha2.ASRInferenceService,
) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: asrSvc.Namespace,
		Name:      asrSvc.Name,
	}, dep); err != nil {
		if apierrors.IsNotFound(err) {
			asrSvc.Status.Replicas = 0
		} else {
			return fmt.Errorf("get Deployment for status sync: %w", err)
		}
	} else {
		asrSvc.Status.Replicas = dep.Status.ReadyReplicas
	}
	asrSvc.Status.ObservedGeneration = asrSvc.Generation
	asrSvc.Status.URL = fmt.Sprintf(
		"http://%s.%s.svc.cluster.local/v1/audio/transcriptions",
		asrSvc.Name, asrSvc.Namespace,
	)

	depReady := dep.Status.ReadyReplicas > 0
	deployCondStatus := metav1.ConditionFalse
	deployCondReason := "Unavailable"
	if depReady {
		deployCondStatus = metav1.ConditionTrue
		deployCondReason = "Available"
	}
	meta.SetStatusCondition(&asrSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.ASRConditionDeploymentReady,
		Status:             deployCondStatus,
		Reason:             deployCondReason,
		ObservedGeneration: asrSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("%d/%d pods ready", dep.Status.ReadyReplicas, dep.Status.Replicas),
	})
	meta.SetStatusCondition(&asrSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.ASRConditionReady,
		Status:             deployCondStatus,
		Reason:             deployCondReason,
		ObservedGeneration: asrSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "ASR service is ready to transcribe audio",
	})

	// Only patch status if it actually changed to avoid infinite reconciliation loops.
	if !equality.Semantic.DeepEqual(&asrSvcBeforePatch.Status, &asrSvc.Status) {
		err := r.Status().Patch(ctx, asrSvc, client.MergeFrom(asrSvcBeforePatch))
		if err != nil {
			if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// setCondition patches a single status condition without a full status sync.
func (r *ASRInferenceServiceReconciler) setCondition(
	ctx context.Context,
	asrSvc *servingv1alpha2.ASRInferenceService,
	asrSvcBeforePatch *servingv1alpha2.ASRInferenceService,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	meta.SetStatusCondition(&asrSvc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: asrSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
	// Only patch status if it actually changed to avoid infinite reconciliation loops.
	if !equality.Semantic.DeepEqual(&asrSvcBeforePatch.Status, &asrSvc.Status) {
		err := r.Status().Patch(ctx, asrSvc, client.MergeFrom(asrSvcBeforePatch))
		if err != nil {
			if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// cleanupASR deletes all owned resources before removing the finalizer.
// Owned Deployment and Service will be garbage-collected by the owner reference;
// this method is a no-op for ASR (no external resources to clean).
func (r *ASRInferenceServiceReconciler) cleanupASR(
	_ context.Context,
	_ *servingv1alpha2.ASRInferenceService,
) error {
	// Deployment and Service carry ownerReferences → deleted automatically.
	return nil
}

// --- builders ---

// buildASRDeployment constructs the desired Deployment for an ASRInferenceService.
func (r *ASRInferenceServiceReconciler) buildASRDeployment(
	asrSvc *servingv1alpha2.ASRInferenceService,
) *appsv1.Deployment {
	labels := asrLabels(asrSvc.Name)
	replicas := ptr.To(int32(1))
	if asrSvc.Spec.Replicas != nil {
		replicas = asrSvc.Spec.Replicas
	}

	// Start from the user-supplied pod template (resources, tolerations, etc.).
	// Spec.Template is an optional pointer; default to an empty template when
	// the user omitted it so the operator can inject the runtime container.
	podTemplate := &corev1.PodTemplateSpec{}
	if asrSvc.Spec.Template != nil {
		podTemplate = asrSvc.Spec.Template.DeepCopy()
	}
	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}
	for k, v := range labels {
		podTemplate.Labels[k] = v
	}
	// Phase 5 Hardening: Enforce default termination grace period
	if podTemplate.Spec.TerminationGracePeriodSeconds == nil {
		grace := r.Defaults.ASRTerminationGracePeriodSeconds
		if grace == 0 {
			grace = ASRTerminationGracePeriod
		}
		podTemplate.Spec.TerminationGracePeriodSeconds = ptr.To(grace)
	}

	// Inject the primary runtime container at position 0.
	serverContainer := r.buildASRContainer(asrSvc)
	podTemplate.Spec.Containers = append([]corev1.Container{serverContainer}, podTemplate.Spec.Containers...)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      asrSvc.Name,
			Namespace: asrSvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: *podTemplate,
		},
	}
}

// buildASRContainer builds the primary runtime container spec.
func (r *ASRInferenceServiceReconciler) buildASRContainer(
	asrSvc *servingv1alpha2.ASRInferenceService,
) corev1.Container {
	container := corev1.Container{
		Name:  asrContainerName,
		Image: asrRuntimeImage(asrSvc),
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: asrServerPort, Protocol: corev1.ProtocolTCP},
		},
		Env:           asrEnvironment(asrSvc),
		LivenessProbe: asrHealthProbe(30, 15, 3),
		// Allow extra time for model download on cold start.
		ReadinessProbe: asrHealthProbe(15, 10, 6),
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(r.asrCPURequest()),
				corev1.ResourceMemory: resource.MustParse(r.asrMemoryRequest()),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(r.asrCPURequest()),
				corev1.ResourceMemory: resource.MustParse(r.asrMemoryRequest()),
			},
		},
	}
	applyAcceleratorResources(&container, asrSvc.Spec.Accelerator)
	return container
}

func (r *ASRInferenceServiceReconciler) asrCPURequest() string {
	if r.Defaults.ASRCPURequest != "" {
		return r.Defaults.ASRCPURequest
	}
	return DefaultASRCPURequest
}

func (r *ASRInferenceServiceReconciler) asrMemoryRequest() string {
	if r.Defaults.ASRMemoryRequest != "" {
		return r.Defaults.ASRMemoryRequest
	}
	return DefaultASRMemoryRequest
}

func asrRuntimeImage(asrSvc *servingv1alpha2.ASRInferenceService) string {
	if asrSvc.Spec.RuntimeImage != "" {
		return asrSvc.Spec.RuntimeImage
	}
	if asrSvc.Spec.Accelerator != nil {
		if image := servingv1alpha2.DefaultASRAcceleratorImage(asrSvc.Spec.Runtime); image != "" {
			return image
		}
	}
	if image := servingv1alpha2.DefaultASRRuntimeImage(asrSvc.Spec.Runtime); image != "" {
		return image
	}
	// ASSUMPTION: caller validated runtime=transformers requires runtimeImage;
	// an empty image here is a programming error, not a user error.
	return "scratch"
}

func asrEnvironment(asrSvc *servingv1alpha2.ASRInferenceService) []corev1.EnvVar {
	var env []corev1.EnvVar
	if asrSvc.Spec.Runtime == servingv1alpha2.ASRRuntimeFasterWhisper {
		env = append(env, corev1.EnvVar{
			Name:  "PRELOAD_MODELS",
			Value: asrPreloadModelsJSON(asrModelID(asrSvc.Spec.Model.URI)),
		})
	} else {
		env = append(env, corev1.EnvVar{Name: "MODEL", Value: asrSvc.Spec.Model.URI})
		if len(asrSvc.Spec.Languages) > 0 {
			env = append(env, corev1.EnvVar{
				Name:  "LANGUAGE",
				Value: strings.Join(asrSvc.Spec.Languages, ","),
			})
		}
	}
	return append(env, corev1.EnvVar{
		Name: "HF_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "hf-credentials"},
				Key:                  "HF_TOKEN",
				Optional:             ptr.To(true),
			},
		},
	})
}

func asrHealthProbe(initialDelay, period, failures int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt32(asrServerPort),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
		FailureThreshold:    failures,
	}
}

// buildASRService constructs the desired ClusterIP Service.
func (r *ASRInferenceServiceReconciler) buildASRService(
	asrSvc *servingv1alpha2.ASRInferenceService,
) *corev1.Service {
	labels := asrLabels(asrSvc.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      asrSvc.Name,
			Namespace: asrSvc.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       asrServerPort,
					TargetPort: intstr.FromInt32(asrServerPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// asrLabels returns the standard label set for ASR workloads.
func asrLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "asrinferenceservice",
		"app.kubernetes.io/instance":  name,
		"app.kubernetes.io/component": "asr-server",
		"app.kubernetes.io/part-of":   "ckodex-llm-operator",
	}
}

func asrModelID(uri string) string {
	return strings.TrimPrefix(
		strings.TrimPrefix(
			strings.TrimPrefix(uri, "hf://"),
			"huggingface://",
		),
		"hf-mirror://",
	)
}

func asrPreloadModelsJSON(modelID string) string {
	return "[" + strconv.Quote(modelID) + "]"
}

// SetupWithManager registers the controller with the manager.
func (r *ASRInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.ASRInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
