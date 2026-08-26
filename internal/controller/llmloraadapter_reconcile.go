package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

func (r *LLMLoraAdapterReconciler) fetchLora(ctx context.Context, req ctrl.Request) (*servingv1alpha2.LLMLoraAdapter, *servingv1alpha2.LLMLoraAdapter, ctrl.Result, error) {
	var lora servingv1alpha2.LLMLoraAdapter
	if err := r.Get(ctx, req.NamespacedName, &lora); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, ctrl.Result{}, nil
		}
		return nil, nil, ctrl.Result{}, err
	}
	return &lora, lora.DeepCopy(), ctrl.Result{}, nil
}

func (r *LLMLoraAdapterReconciler) prepareLora(ctx context.Context, lora, original *servingv1alpha2.LLMLoraAdapter) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if err := r.initializeLoraState(ctx, lora, original); err != nil {
		return nil, err
	}
	if lora.Status.StatePlanes.Lifecycle == "quarantined" {
		logger.Info("Adapter is quarantined, blocking load", "Adapter", lora.Name)
		if err := r.unloadFromTargetService(ctx, lora); err != nil {
			logger.Error(err, "Failed to unload quarantined adapter; load remains blocked", "adapter", lora.Name, "namespace", lora.Namespace, "targetService", lora.Spec.TargetService)
		}
		r.Recorder.Event(lora, corev1.EventTypeWarning, "Quarantined", "Access to this composite model is forcibly blocked due to governance failure")
		observability.QuarantineIncidents.WithLabelValues(lora.Name, "manual_quarantine").Inc()
		observability.GovernanceState.WithLabelValues("quarantined", lora.Status.StatePlanes.Trust).Set(1)
		return &ctrl.Result{}, nil
	}
	if err := r.ensureLoraFinalizer(ctx, lora); err != nil {
		return nil, err
	}
	return nil, r.ensureProgressingCondition(ctx, lora)
}

func (r *LLMLoraAdapterReconciler) initializeLoraState(ctx context.Context, lora, original *servingv1alpha2.LLMLoraAdapter) error {
	if lora.Status.StatePlanes.Lifecycle != "" {
		return nil
	}
	lora.Status.StatePlanes.Lifecycle = "proposed"
	lora.Status.StatePlanes.Trust = "unknown"
	lora.Status.StatePlanes.Risk = "normal"
	return r.Status().Patch(ctx, lora, client.MergeFrom(original))
}

func (r *LLMLoraAdapterReconciler) ensureLoraFinalizer(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	if containsString(lora.Finalizers, loraFinalizer) {
		return nil
	}
	lora.Finalizers = append(lora.Finalizers, loraFinalizer)
	return r.Update(ctx, lora)
}

func (r *LLMLoraAdapterReconciler) ensureProgressingCondition(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) error {
	for _, condition := range lora.Status.Conditions {
		if condition.Type == "Progressing" || condition.Type == servingv1alpha2.AdapterConditionReady {
			return nil
		}
	}
	patch := client.MergeFrom(lora.DeepCopy())
	lora.Status.Conditions = append(lora.Status.Conditions, metav1.Condition{
		Type: "Progressing", Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "LoRA hot-swap reconciliation started", LastTransitionTime: metav1.Now(),
	})
	return r.Status().Patch(ctx, lora, patch)
}

func (r *LLMLoraAdapterReconciler) ensureLoraCache(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter) (*servingv1alpha2.LocalModelCache, *ctrl.Result, error) {
	logger := log.FromContext(ctx)
	expected := newLoraCache(lora)
	var existing servingv1alpha2.LocalModelCache
	err := r.Get(ctx, client.ObjectKey{Name: expected.Name}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating LocalModelCache for LoRA adapter", "Name", expected.Name)
		if err := r.Create(ctx, expected); err != nil {
			return nil, nil, err
		}
		return nil, resultAfter(time.Second), nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := validateLoraCacheOwner(&existing, lora); err != nil {
		return nil, nil, err
	}
	if !loraCacheReady(&existing) {
		logger.Info("Waiting for LoRA LocalModelCache to finish downloading", "Name", expected.Name)
		return nil, resultAfter(5 * time.Second), nil
	}
	return &existing, nil, nil
}

func resultAfter(delay time.Duration) *ctrl.Result {
	return &ctrl.Result{RequeueAfter: delay}
}

func loraCacheReady(cache *servingv1alpha2.LocalModelCache) bool {
	for _, condition := range cache.Status.Conditions {
		if condition.Type == servingv1alpha2.ConditionReady && condition.Status == "True" {
			return true
		}
	}
	return false
}

func (r *LLMLoraAdapterReconciler) hydrateAndGovernLora(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, cache *servingv1alpha2.LocalModelCache) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if updated, err := r.hydrateVerificationEvidence(ctx, lora, cache); err != nil {
		logger.Error(err, "Failed to read LoRA runtime verification evidence", "adapter", lora.Name)
	} else if updated {
		if err := r.Status().Update(ctx, lora); err != nil {
			return nil, err
		}
	}
	engine := governance.NewDefaultEngine()
	valid, reason := engine.Check(ctx, lora)
	if !valid {
		if err := r.quarantineGovernanceFailure(ctx, lora, reason); err != nil {
			return nil, err
		}
		return &ctrl.Result{}, nil
	}
	if lora.Status.StatePlanes.Lifecycle != "active" {
		patch := client.MergeFrom(lora.DeepCopy())
		governance.TransitionStates(lora, true, "")
		r.Recorder.Eventf(lora, corev1.EventTypeNormal, "GovernancePass", "Conformance vectors passed. Adapter remains %s while stronger verification is pending.", lora.Status.StatePlanes.Trust)
		observability.GovernanceState.WithLabelValues(lora.Status.StatePlanes.Lifecycle, lora.Status.StatePlanes.Trust).Set(1)
		return nil, r.Status().Patch(ctx, lora, patch)
	}
	return nil, nil
}

func (r *LLMLoraAdapterReconciler) quarantineGovernanceFailure(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, reason string) error {
	logger := log.FromContext(ctx)
	logger.Error(nil, "Governance Check Failed", "Reason", reason)
	governance.TransitionStates(lora, false, reason)
	r.Recorder.Eventf(lora, corev1.EventTypeWarning, "GovernanceFail", "Adapter failed conformance vectors: %s", reason)
	observability.QuarantineIncidents.WithLabelValues(lora.Name, reason).Inc()
	observability.GovernanceState.WithLabelValues("quarantined", "denied").Set(1)
	return r.Status().Update(ctx, lora)
}
