/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
	"github.com/ckodex-labs/kserve-llm-operator/internal/scheduler"
)

func (r *LLMInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.APIReader = mgr.GetAPIReader()
	r.initializeDeploymentComponents(mgr)
	r.initializeServiceComponents(mgr)
	r.initializeRuntimeComponents(mgr)
	r.Recorder = mgr.GetEventRecorderFor("ckodex-llm-operator")
	return r.setupManagedController(mgr)
}

func (r *LLMInferenceServiceReconciler) initializeDeploymentComponents(mgr ctrl.Manager) {
	r.DeploymentBuilder = &deployment.Builder{
		Client: mgr.GetClient(), Recorder: mgr.GetEventRecorderFor("ckodex-llm-operator"), SPIRE: r.SPIRE,
		EnableHardwareSelection: r.EnableHardwareSelection, OTEL_Endpoint: r.OTEL_Endpoint,
		AirGappedMode: r.AirGappedMode, LocalRegistry: r.LocalRegistry, LocalCosignKeyPath: r.LocalCosignKeyPath,
		RuntimeImage: r.RuntimeImage, HFInitializerImage: r.HFInitializerImage, HFMirrorURL: r.HFMirrorURL,
		Defaults: r.Defaults,
	}
	r.StatusReconciler = &status.Reconciler{Client: mgr.GetClient(), EnableHardening: r.EnableExperimentalStatusHardening}
	r.CleanupReconciler = &cleanup.Reconciler{Client: mgr.GetClient()}
}

func (r *LLMInferenceServiceReconciler) initializeServiceComponents(mgr ctrl.Manager) {
	r.ServiceReconciler = &reconciler.ServiceReconciler{Client: mgr.GetClient(), Scheme: r.Scheme, EnableGRPC: r.EnableGRPC}
	r.PDBReconciler = &reconciler.PDBReconciler{Client: mgr.GetClient(), Scheme: r.Scheme}
	r.GovernanceReconciler = &evidence.GovernanceReconciler{
		Client: mgr.GetClient(), Scheme: r.Scheme, AirGappedMode: r.AirGappedMode, LocalCosignKeyPath: r.LocalCosignKeyPath,
	}
}

func (r *LLMInferenceServiceReconciler) initializeRuntimeComponents(mgr ctrl.Manager) {
	r.HFCSI = &HFCSIReconciler{Client: mgr.GetClient(), Scheme: r.Scheme}
	r.KServeMultiNode = &kserveintegration.Reconciler{Client: mgr.GetClient(), Scheme: r.Scheme, RuntimeName: r.KServeMultiNodeRuntime}
}

func (r *LLMInferenceServiceReconciler) setupManagedController(mgr ctrl.Manager) error {
	inferencePool := &unstructured.Unstructured{}
	inferencePool.SetGroupVersionKind(scheduler.InferencePoolGVK)
	builder := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		For(&servingv1alpha2.LLMInferenceService{}).
		Owns(&appsv1.Deployment{}).Owns(&corev1.Service{}).Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&corev1.PersistentVolumeClaim{}).Owns(&gwapiv1.HTTPRoute{}).Owns(&gwapiv1.GRPCRoute{}).
		Owns(&gwapiv1.Gateway{})
	if _, err := mgr.GetRESTMapper().RESTMapping(scheduler.InferencePoolGVK.GroupKind(), scheduler.InferencePoolGVK.Version); err == nil {
		builder = builder.Owns(inferencePool)
	} else if !meta.IsNoMatchError(err) {
		return fmt.Errorf("discover InferencePool CRD: %w", err)
	}
	return builder.
		Watches(&servingv1alpha2.LocalModelCache{}, handler.EnqueueRequestsFromMapFunc(r.mapLocalModelCacheToInferenceServices)).
		Watches(&servingv1alpha2.LLMLoraAdapter{}, handler.EnqueueRequestsFromMapFunc(r.mapLoraAdapterToInferenceService)).
		Complete(r)
}
