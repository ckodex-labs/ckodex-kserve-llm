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
)

const (
	// embeddingFinalizer is the finalizer the Embedding controller registers on each CR.
	embeddingFinalizer = "serving.ckodex.com/embedding-finalizer"

	// embeddingContainerName is the name of the primary serving container.
	embeddingContainerName = "embedding-server"
)

// EmbeddingInferenceServiceReconciler reconciles EmbeddingInferenceService objects.
// It owns: Deployment, Service.
// It does NOT own: Gateway, HTTPRoute — embedding services are cluster-internal by default.
// External exposure is left to the user (Ingress/Gateway).
type EmbeddingInferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=embeddinginferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=embeddinginferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=embeddinginferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the main reconcile loop.
func (r *EmbeddingInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var embSvc servingv1alpha2.EmbeddingInferenceService
	if err := r.Get(ctx, req.NamespacedName, &embSvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch EmbeddingInferenceService: %w", err)
	}

	// Handle deletion.
	if embSvc.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&embSvc, embeddingFinalizer) {
			controllerutil.RemoveFinalizer(&embSvc, embeddingFinalizer)
			if err := r.Update(ctx, &embSvc); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove embedding finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Register finalizer.
	if !controllerutil.ContainsFinalizer(&embSvc, embeddingFinalizer) {
		controllerutil.AddFinalizer(&embSvc, embeddingFinalizer)
		if err := r.Update(ctx, &embSvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add embedding finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Reconcile Deployment.
	if err := r.reconcileEmbeddingDeployment(ctx, &embSvc); err != nil {
		_ = r.setEmbeddingCondition(ctx, &embSvc, servingv1alpha2.EmbeddingConditionDeploymentReady,
			metav1.ConditionFalse, "ReconcileError", err.Error())
		return ctrl.Result{}, fmt.Errorf("reconcile Embedding Deployment: %w", err)
	}

	// Reconcile Service.
	if err := r.reconcileEmbeddingService(ctx, &embSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile Embedding Service: %w", err)
	}

	// Sync status.
	if err := r.syncEmbeddingStatus(ctx, &embSvc); err != nil {
		return ctrl.Result{}, fmt.Errorf("sync Embedding status: %w", err)
	}

	logger.Info("EmbeddingInferenceService reconciled", "name", embSvc.Name, "runtime", embSvc.Spec.Runtime)
	return ctrl.Result{}, nil
}

func (r *EmbeddingInferenceServiceReconciler) reconcileEmbeddingDeployment(
	ctx context.Context,
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) error {
	desired := r.buildEmbeddingDeployment(embSvc)
	if err := controllerutil.SetControllerReference(embSvc, desired, r.Scheme); err != nil {
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

func (r *EmbeddingInferenceServiceReconciler) reconcileEmbeddingService(
	ctx context.Context,
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) error {
	desired := r.buildEmbeddingService(embSvc)
	if err := controllerutil.SetControllerReference(embSvc, desired, r.Scheme); err != nil {
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
	// ClusterIP is immutable — update selector and ports only.
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

func (r *EmbeddingInferenceServiceReconciler) syncEmbeddingStatus(
	ctx context.Context,
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: embSvc.Namespace,
		Name:      embSvc.Name,
	}, dep); err != nil {
		return fmt.Errorf("get Deployment for status sync: %w", err)
	}

	patch := client.MergeFrom(embSvc.DeepCopy())

	embSvc.Status.Replicas = dep.Status.ReadyReplicas
	embSvc.Status.ObservedGeneration = embSvc.Generation
	embSvc.Status.URL = fmt.Sprintf(
		"http://%s.%s.svc.cluster.local/v1/embeddings",
		embSvc.Name, embSvc.Namespace,
	)

	depReady := dep.Status.ReadyReplicas > 0
	condStatus := metav1.ConditionFalse
	condReason := "Unavailable"
	if depReady {
		condStatus = metav1.ConditionTrue
		condReason = "Available"
	}

	meta.SetStatusCondition(&embSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.EmbeddingConditionDeploymentReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: embSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("%d/%d pods ready", dep.Status.ReadyReplicas, dep.Status.Replicas),
	})
	meta.SetStatusCondition(&embSvc.Status.Conditions, metav1.Condition{
		Type:               servingv1alpha2.EmbeddingConditionReady,
		Status:             condStatus,
		Reason:             condReason,
		ObservedGeneration: embSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "Embedding service is ready to produce vectors",
	})

	return r.Status().Patch(ctx, embSvc, patch)
}

func (r *EmbeddingInferenceServiceReconciler) setEmbeddingCondition(
	ctx context.Context,
	embSvc *servingv1alpha2.EmbeddingInferenceService,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	patch := client.MergeFrom(embSvc.DeepCopy())
	meta.SetStatusCondition(&embSvc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: embSvc.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            message,
	})
	return r.Status().Patch(ctx, embSvc, patch)
}

// --- builders ---

// buildEmbeddingDeployment constructs the desired Deployment.
func (r *EmbeddingInferenceServiceReconciler) buildEmbeddingDeployment(
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) *appsv1.Deployment {
	labels := embeddingLabels(embSvc.Name)
	replicas := ptr.To(int32(1))
	if embSvc.Spec.Replicas != nil {
		replicas = embSvc.Spec.Replicas
	}

	podTemplate := embSvc.Spec.Template.DeepCopy()
	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}
	maps.Copy(podTemplate.Labels, labels)

	serverContainer := r.buildEmbeddingContainer(embSvc)
	podTemplate.Spec.Containers = append([]corev1.Container{serverContainer}, podTemplate.Spec.Containers...)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      embSvc.Name,
			Namespace: embSvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: *podTemplate,
		},
	}
}

// buildEmbeddingContainer builds the primary runtime container spec.
// Both infinity and TEI use CLI flags to configure the model and port.
func (r *EmbeddingInferenceServiceReconciler) buildEmbeddingContainer(
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) corev1.Container {
	img := embSvc.Spec.RuntimeImage
	if img == "" {
		img = servingv1alpha2.DefaultEmbeddingRuntimeImage(embSvc.Spec.Runtime)
	}
	if img == "" {
		img = servingv1alpha2.DefaultEmbeddingRuntimeImage(servingv1alpha2.EmbeddingRuntimeInfinity)
	}

	// FACT: both runtimes accept the model as a CLI argument.
	// Strip hf:// prefix to get the raw HuggingFace model ID.
	modelID := embeddingModelID(embSvc.Spec.Model.URI)
	portStr := fmt.Sprintf("%d", servingv1alpha2.EmbeddingServerPort)

	var args []string
	switch embSvc.Spec.Runtime {
	case servingv1alpha2.EmbeddingRuntimeTextEmbeddingsInference:
		// TEI entrypoint: text-embeddings-router --model-id <id> --port 7997
		args = []string{
			"--model-id", modelID,
			"--port", portStr,
		}
	default:
		// infinity: infinity_emb v2 --model-name-or-path <id> --port 7997
		batchSize := int32(32)
		if embSvc.Spec.BatchSize != nil {
			batchSize = *embSvc.Spec.BatchSize
		}
		args = []string{
			"v2",
			"--model-name-or-path", modelID,
			"--port", portStr,
			"--batch-size", fmt.Sprintf("%d", batchSize),
		}
	}

	// Inject HF_TOKEN from canonical secret (optional — public models do not require it).
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

	// Storage secret — passed as HUGGING_FACE_HUB_TOKEN alias for TEI.
	if embSvc.Spec.Model.Storage != nil && embSvc.Spec.Model.Storage.SecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "HUGGING_FACE_HUB_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: embSvc.Spec.Model.Storage.SecretRef.Name},
					Key:                  "HF_TOKEN",
					Optional:             ptr.To(true),
				},
			},
		})
	}

	port := int32(servingv1alpha2.EmbeddingServerPort)
	return corev1.Container{
		Name:  embeddingContainerName,
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
			InitialDelaySeconds: 30,
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
			InitialDelaySeconds: 15,
			PeriodSeconds:       10,
			FailureThreshold:    6,
		},
	}
}

// buildEmbeddingService constructs the desired ClusterIP Service.
func (r *EmbeddingInferenceServiceReconciler) buildEmbeddingService(
	embSvc *servingv1alpha2.EmbeddingInferenceService,
) *corev1.Service {
	labels := embeddingLabels(embSvc.Name)
	port := int32(servingv1alpha2.EmbeddingServerPort)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      embSvc.Name,
			Namespace: embSvc.Namespace,
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

// embeddingLabels returns the standard label set for embedding workloads.
func embeddingLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "embeddinginferenceservice",
		"app.kubernetes.io/instance":  name,
		"app.kubernetes.io/component": "embedding-server",
		"app.kubernetes.io/part-of":   "ckodex-llm-operator",
	}
}

// embeddingModelID strips the hf:// URI scheme prefix to get the raw HuggingFace model ID.
// For non-hf:// URIs, the full URI is returned as-is.
func embeddingModelID(uri string) string {
	id, ok := strings.CutPrefix(uri, "hf://")
	if ok {
		return id
	}
	return uri
}

// SetupWithManager registers the controller with the manager.
func (r *EmbeddingInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.EmbeddingInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
