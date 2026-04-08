/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// AgentReconciler reconciles Agent objects.
// It validates that the referenced LLMInferenceService exists and is Ready,
// then marks the Agent as Ready. When the model is unavailable the Agent
// degrades to Pending so callers can detect unhealthy bindings.
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llminferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=skillregistries,verbs=get;list;watch

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var agent servingv1alpha2.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get agent: %w", err)
	}

	// Capture original object for diffing and patching
	agentBeforePatch := agent.DeepCopy()

	// Handle deletion
	if agent.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&agent, FinalizerName) {
			controllerutil.RemoveFinalizer(&agent, FinalizerName)
			if err := r.Update(ctx, &agent); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&agent, FinalizerName) {
		controllerutil.AddFinalizer(&agent, FinalizerName)
		if err := r.Update(ctx, &agent); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Validate referenced LLMInferenceService
	modelReady, msg, err := r.validateModelRef(ctx, &agent)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("validate modelRef: %w", err)
	}

	// Validate skill registry references
	registriesReady, registryMsg, err := r.validateSkillRefs(ctx, &agent)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("validate skill refs: %w", err)
	}

	// Compute status
	ready := modelReady && registriesReady
	reason := "AgentReady"
	message := "Agent is bound and all references are valid"
	if !modelReady {
		reason = "ModelNotReady"
		message = msg
	} else if !registriesReady {
		reason = "SkillRegistryNotReady"
		message = registryMsg
	}

	// Update status conditions
	meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             boolToConditionStatus(ready),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: agent.Generation,
	})
	agent.Status.Ready = ready

	// Only patch status if it actually changed to avoid infinite reconciliation loops
	if !equality.Semantic.DeepEqual(&agentBeforePatch.Status, &agent.Status) {
		if err := r.Status().Patch(ctx, &agent, client.MergeFrom(agentBeforePatch)); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch agent status: %w", err)
		}
	}

	logger.Info("agent reconciled", "name", agent.Name, "ready", ready)
	return ctrl.Result{}, nil
}

// validateModelRef checks that the referenced LLMInferenceService exists and is Ready.
func (r *AgentReconciler) validateModelRef(ctx context.Context, agent *servingv1alpha2.Agent) (bool, string, error) {
	if agent.Spec.ModelRef == "" {
		return false, "spec.modelRef is required", nil
	}

	var llmSvc servingv1alpha2.LLMInferenceService
	key := types.NamespacedName{Name: agent.Spec.ModelRef, Namespace: agent.Namespace}
	if err := r.Get(ctx, key, &llmSvc); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("LLMInferenceService %q not found", agent.Spec.ModelRef), nil
		}
		return false, "", fmt.Errorf("get LLMInferenceService: %w", err)
	}

	if !llmSvc.Status.ModelReady {
		return false, fmt.Sprintf("LLMInferenceService %q is not ready yet", agent.Spec.ModelRef), nil
	}
	return true, "", nil
}

// validateSkillRefs checks that all referenced SkillRegistries exist.
func (r *AgentReconciler) validateSkillRefs(ctx context.Context, agent *servingv1alpha2.Agent) (bool, string, error) {
	for _, ref := range agent.Spec.Skills {
		var reg servingv1alpha2.SkillRegistry
		key := types.NamespacedName{Name: ref.RegistryRef, Namespace: agent.Namespace}
		if err := r.Get(ctx, key, &reg); err != nil {
			if apierrors.IsNotFound(err) {
				return false, fmt.Sprintf("SkillRegistry %q not found", ref.RegistryRef), nil
			}
			return false, "", fmt.Errorf("get SkillRegistry: %w", err)
		}
		// Verify skill name exists in registry
		found := false
		for _, entry := range reg.Spec.Entries {
			if entry.Name == ref.SkillName {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("skill %q not found in SkillRegistry %q", ref.SkillName, ref.RegistryRef), nil
		}
	}
	return true, "", nil
}

// boolToConditionStatus converts a bool to a metav1.ConditionStatus.
func boolToConditionStatus(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// SetupWithManager registers the AgentReconciler with the controller manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		For(&servingv1alpha2.Agent{}).
		Named("agent").
		Complete(r)
}
