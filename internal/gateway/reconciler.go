/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"context"
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
)

// Reconciler manages Gateway, HTTPRoute, and GRPCRoute resources.
// It is a helper called by the main LLMInferenceServiceReconciler,
// not a standalone controller-runtime reconciler.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	EnableGRPC bool // When true, adds gRPC listener and reconciles GRPCRoute
}

// Reconcile creates/updates Gateway API resources for the given LLMInferenceService.
func (r *Reconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "gateway")

	// 1. Reconcile Gateway (managed mode)
	if llmSvc.Spec.Router.Gateway.Managed != nil {
		if err := r.reconcileGateway(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile gateway: %w", err)
		}
		logger.Info("gateway reconciled")
	}

	// 2. Reconcile HTTPRoute (V2 + OpenAI + Embeddings)
	if err := r.reconcileHTTPRoute(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile httproute: %w", err)
	}
	logger.Info("httproute reconciled")

	// 3. Reconcile GRPCRoute (V2 gRPC methods) — gated behind EnableGRPC.
	// vLLM's OpenAI-compatible server does not expose a separate gRPC listener,
	// so this is disabled by default. Enable via CKODEX_FEATURE_ENABLE_GRPC=true
	// when using a backend that serves the V2 gRPC Inference Protocol (e.g., Triton).
	if r.EnableGRPC && !kserveintegration.RequiresMultiNode(llmSvc) {
		if err := r.reconcileGRPCRoute(ctx, llmSvc); err != nil {
			return fmt.Errorf("reconcile grpcroute: %w", err)
		}
		logger.Info("grpcroute reconciled")
	}

	return nil
}

// reconcileGateway creates or updates the managed Gateway resource.
func (r *Reconciler) reconcileGateway(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	gatewayClassName := "envoy"
	if llmSvc.Spec.Router.Gateway.Managed != nil && llmSvc.Spec.Router.Gateway.Managed.GatewayClassName != "" {
		gatewayClassName = llmSvc.Spec.Router.Gateway.Managed.GatewayClassName
	}

	className := gwapiv1.ObjectName(gatewayClassName)
	listeners := []gwapiv1.Listener{
		{
			Name:     "http",
			Port:     80,
			Protocol: gwapiv1.HTTPProtocolType,
		},
	}
	if r.EnableGRPC {
		listeners = append(listeners, gwapiv1.Listener{
			Name:     "grpc",
			Port:     8001,
			Protocol: gwapiv1.HTTPProtocolType, // HTTPProtocolType is correct per Gateway API spec: GRPCRoute attachment signals gRPC semantics to the data plane (Envoy). Cleartext gRPC (h2c) uses HTTP listeners.
		})
	}

	desired := &gwapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-gateway",
			Namespace: llmSvc.Namespace,
			Labels:    commonLabels(llmSvc),
		},
		Spec: gwapiv1.GatewaySpec{
			GatewayClassName: className,
			Listeners:        listeners,
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing gwapiv1.Gateway
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if apiequality.Semantic.DeepEqual(&existing, original) {
		return nil
	}
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

// reconcileHTTPRoute creates or updates the HTTPRoute. When spec.canary is set,
// a weighted two-backend route is built; otherwise the standard single-backend
// route is used.
func (r *Reconciler) reconcileHTTPRoute(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "gateway")

	// Fetch all LoRA adapters for sandbox rules (M3 Phase 3)
	var adapters servingv1alpha2.LLMLoraAdapterList
	if err := r.List(ctx, &adapters, client.InNamespace(llmSvc.Namespace)); err != nil {
		return fmt.Errorf("list adapters: %w", err)
	}

	var associated []servingv1alpha2.LLMLoraAdapter
	for _, a := range adapters.Items {
		if a.Spec.TargetService == llmSvc.Name {
			associated = append(associated, a)
		}
	}

	var httpRoute *gwapiv1.HTTPRoute
	if llmSvc.Spec.Canary != nil {
		httpRoute = BuildCanaryHTTPRoute(llmSvc, associated)
		logger.Info("canary httproute selected",
			"weight", llmSvc.Spec.Canary.Weight,
			"baseModel", llmSvc.Spec.Canary.BaseModel,
		)
	} else {
		httpRoute = BuildHTTPRoute(llmSvc, associated)
	}

	if err := controllerutil.SetControllerReference(llmSvc, httpRoute, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing gwapiv1.HTTPRoute
	err := r.Get(ctx, types.NamespacedName{Name: httpRoute.Name, Namespace: httpRoute.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, httpRoute)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Spec = httpRoute.Spec
	existing.Labels = httpRoute.Labels
	existing.Annotations = httpRoute.Annotations
	if apiequality.Semantic.DeepEqual(&existing, original) {
		return nil
	}
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

// reconcileGRPCRoute creates GRPCRoute with V2 gRPC method matching.
func (r *Reconciler) reconcileGRPCRoute(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	grpcRoute := BuildGRPCRoute(llmSvc)

	if err := controllerutil.SetControllerReference(llmSvc, grpcRoute, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing gwapiv1.GRPCRoute
	err := r.Get(ctx, types.NamespacedName{Name: grpcRoute.Name, Namespace: grpcRoute.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, grpcRoute)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Spec = grpcRoute.Spec
	if apiequality.Semantic.DeepEqual(&existing, original) {
		return nil
	}
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

// GatewayRef returns the parent reference for routes to use.
func GatewayRef(llmSvc *servingv1alpha2.LLMInferenceService) gwapiv1.ParentReference {
	if llmSvc.Spec.Router.Gateway.ExistingRef != nil {
		ns := gwapiv1.Namespace(llmSvc.Spec.Router.Gateway.ExistingRef.Namespace)
		return gwapiv1.ParentReference{
			Name:      gwapiv1.ObjectName(llmSvc.Spec.Router.Gateway.ExistingRef.Name),
			Namespace: &ns,
		}
	}
	return gwapiv1.ParentReference{
		Name: gwapiv1.ObjectName(llmSvc.Name + "-gateway"),
	}
}

func commonLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     strings.ReplaceAll(llmSvc.Spec.Model.Name, "/", "."),
	}
}
