package controller

import (
	"context"
	"fmt"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// checkGateCriteria validates promotion gate metrics against live Prometheus data.
func (r *ModelOnboardingReconciler) checkGateCriteria(
	ctx context.Context,
	_ *servingv1alpha2.ModelOnboarding,
	llmSvc *servingv1alpha2.LLMInferenceService,
	gate *servingv1alpha2.GateCriteria,
) error {
	if err := validateGateReadiness(llmSvc, gate); err != nil {
		return err
	}
	q := r.metricsQuerier()
	if q == nil {
		return fmt.Errorf("gate: promotion metrics backend is not configured")
	}
	if err := checkGateSuccessRate(ctx, q, llmSvc, gate); err != nil {
		return err
	}
	return checkGateLatency(ctx, q, llmSvc, gate)
}

func validateGateReadiness(
	llmSvc *servingv1alpha2.LLMInferenceService,
	gate *servingv1alpha2.GateCriteria,
) error {
	if llmSvc.Status.Replicas < 1 {
		return fmt.Errorf("gate: no ready replicas (minSuccessRate=%d%%)", gate.MinSuccessRate)
	}
	if !llmSvc.Status.ModelReady {
		return fmt.Errorf("gate: model not ready (minSuccessRate=%d%%)", gate.MinSuccessRate)
	}
	return nil
}

func checkGateSuccessRate(
	ctx context.Context,
	q MetricsQuerier,
	llmSvc *servingv1alpha2.LLMInferenceService,
	gate *servingv1alpha2.GateCriteria,
) error {
	successRate, err := q.QuerySuccessRate(ctx, llmSvc.Spec.Model.Name, llmSvc.Namespace)
	if err != nil {
		return fmt.Errorf("gate: metrics unavailable: %w", err)
	}
	if int32(successRate) < gate.MinSuccessRate {
		return fmt.Errorf("gate: success rate %.1f%% < required %d%%", successRate, gate.MinSuccessRate)
	}
	return nil
}

func checkGateLatency(
	ctx context.Context,
	q MetricsQuerier,
	llmSvc *servingv1alpha2.LLMInferenceService,
	gate *servingv1alpha2.GateCriteria,
) error {
	if gate.MaxLatencyP99 == nil || *gate.MaxLatencyP99 <= 0 {
		return nil
	}
	p99MS, err := q.QueryP99LatencyMS(ctx, llmSvc.Spec.Model.Name, llmSvc.Namespace)
	if err != nil {
		return fmt.Errorf("gate: P99 latency metrics unavailable: %w", err)
	}
	if p99MS > *gate.MaxLatencyP99 {
		return fmt.Errorf("gate: P99 latency %dms > max allowed %dms", p99MS, *gate.MaxLatencyP99)
	}
	return nil
}
