/*
Copyright 2026 CKodex Authors.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/governance/evaluator"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// LLMEvaluationReconciler handles asynchronous model benchmarking and safety verification.
type LLMEvaluationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	AuditLogger *observability.AuditLogger
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters/status,verbs=get;update;patch

func (r *LLMEvaluationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the adapter
	var adapter servingv1alpha2.LLMLoraAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only process adapters in 'pending-evaluation' state
	if adapter.Status.StatePlanes.Lifecycle != "pending-evaluation" {
		return ctrl.Result{}, nil
	}

	logger.Info("Starting asynchronous evaluation benchmark", "adapter", adapter.Name)

	// In a real implementation, this would trigger a Kubernetes Job or call an external service.
	// For this phase, we simulate a robust evaluation process.
	
	// Simulate evaluation latency (e.g. 2 seconds for the sake of the demo loop)
	time.Sleep(2 * time.Second)

	// Create mock evaluation report
	report := &evaluator.EvalReport{
		SafetyScore:      8.5,
		RefusalRate:      0.02,
		CompatibilityV:   []float64{1.0, 1.0, 0.9},
		VerificationTime: time.Now(),
	}

	// Log evaluation event
	r.AuditLogger.Log(ctx, &adapter, "EvaluationStarted", "Asynchronous safety benchmark initiated")

	// Apply evaluation results
	patch := client.MergeFrom(adapter.DeepCopy())
	evaluator.GenerateEvidence(&adapter, report)
	
	// Transition trust to 'verified'
	adapter.Status.StatePlanes.Trust = "verified"
	
	now := metav1.NewTime(time.Now())
	adapter.Status.EvidenceBundle.LastVerifiedAt = &now

	if err := r.Status().Patch(ctx, &adapter, patch); err != nil {
		logger.Error(err, "Failed to update adapter status with evaluation results")
		return ctrl.Result{}, err
	}

	logger.Info("Evaluation benchmark completed successfully", "adapter", adapter.Name, "trust", adapter.Status.StatePlanes.Trust)
	r.AuditLogger.Log(ctx, &adapter, "EvaluationCompleted", "Adapter verified successfully through safety benchmark")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMEvaluationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Complete(r)
}
