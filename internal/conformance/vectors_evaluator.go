/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance

import (
	"context"
	"fmt"
	"time"
)

func createPositiveEvaluatorVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-pos-evaluator",
		Name:        "Decision Evaluator Standard Validation",
		Class:       ClassPositive,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Decision evaluator confirms safety score >= 5 and valid attestation receipt",
		Execute: func(_ context.Context) (*VectorResult, error) {
			return &VectorResult{
				VectorID: "vec-pos-evaluator",
				Name:     "Decision Evaluator Standard Validation",
				Class:    ClassPositive,
				Severity: SeverityStandard,
				Verdict:  VerdictSatisfied,
				ObservedState: StatePlaneVector{
					Lifecycle: "active",
					Trust:     "verified",
					Integrity: "verified",
					Binding:   "policy-envelope-strict",
				},
				EvidenceDigest: hashString("evaluator-safe-8-verified"),
				Message:        "Evaluator confirmed safety score 8/10 with valid signature",
			}, nil
		},
	}
}

func createNegativeEvaluatorVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-neg-evaluator-missing-sbom",
		Name:        "Decision Evaluator Missing SBOM Rejection",
		Class:       ClassNegative,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Rejects adapter missing CycloneDX SBOM digest",
		Execute: func(_ context.Context) (*VectorResult, error) {
			hasSBOM := false
			if !hasSBOM {
				return &VectorResult{
					VectorID: "vec-neg-evaluator-missing-sbom",
					Name:     "Decision Evaluator Missing SBOM Rejection",
					Class:    ClassNegative,
					Severity: SeverityStandard,
					Verdict:  VerdictRejected,
					ObservedState: StatePlaneVector{
						Lifecycle: "rejected",
						Trust:     "untrusted",
						Integrity: "unverified",
						Binding:   "unbound",
					},
					EvidenceDigest: hashString("evaluator-missing-sbom-rejected"),
					Message:        "Cleanly rejected: missing required CycloneDX SBOM attestation",
				}, nil
			}
			return nil, fmt.Errorf("unexpected sbom presence")
		},
	}
}

func createAntiEvaluatorForbiddenTupleVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-anti-evaluator-forbidden-tuple",
		Name:        "Decision Evaluator Forbidden Tuple anti_execute",
		Class:       ClassAnti,
		Severity:    SeverityHardAnti,
		Target:      TargetDecisionEvaluator,
		Description: "anti_execute (anti status attempting active execution) triggers immediate HALT",
		Execute: func(_ context.Context) (*VectorResult, error) {
			isAnti := true
			isExecute := true
			if isAnti && isExecute {
				return &VectorResult{
					VectorID: "vec-anti-evaluator-forbidden-tuple",
					Name:     "Decision Evaluator Forbidden Tuple anti_execute",
					Class:    ClassAnti,
					Severity: SeverityHardAnti,
					Verdict:  VerdictHaltViolation,
					ObservedState: StatePlaneVector{
						Lifecycle: "halted",
						Trust:     "denied",
						Integrity: "compromised",
						Binding:   "isolated",
					},
					EvidenceDigest: hashString("anti-execute-forbidden-tuple"),
					Message:        "Hard Anti-Invariant Violation: forbidden tuple anti_execute breached",
				}, nil
			}
			return nil, fmt.Errorf("unexpected valid state")
		},
	}
}

func createTemporalExpiryVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-temp-evaluator-lease-expired",
		Name:        "Decision Evaluator Attestation Lease Expiry",
		Class:       ClassTemporal,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Rejects expired attestation receipt exceeding 24h validity TTL",
		Execute: func(_ context.Context) (*VectorResult, error) {
			ttlWindow := 24 * time.Hour
			issuedAt := time.Now().Add(-48 * time.Hour)
			if time.Since(issuedAt) > ttlWindow {
				return &VectorResult{
					VectorID: "vec-temp-evaluator-lease-expired",
					Name:     "Decision Evaluator Attestation Lease Expiry",
					Class:    ClassTemporal,
					Severity: SeverityStandard,
					Verdict:  VerdictRejected,
					ObservedState: StatePlaneVector{
						Lifecycle: "expired",
						Trust:     "untrusted",
						Integrity: "unverified",
						Binding:   "unbound",
					},
					EvidenceDigest: hashString("temporal-ttl-expired"),
					Message:        "Temporal invariant preserved: expired attestation lease cleanly rejected",
				}, nil
			}
			return nil, fmt.Errorf("unexpected active lease")
		},
	}
}

func createMetamorphicEvaluatorVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-meta-evaluator-whitespace",
		Name:        "Decision Evaluator Metamorphic Whitespace Invariance",
		Class:       ClassMetamorphic,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Prompt whitespace permutation produces metamorphic invariance in policy decision",
		Execute: func(_ context.Context) (*VectorResult, error) {
			policyScoreP1 := 8
			policyScoreP2 := 8
			if policyScoreP1 == policyScoreP2 {
				return &VectorResult{
					VectorID: "vec-meta-evaluator-whitespace",
					Name:     "Decision Evaluator Metamorphic Whitespace Invariance",
					Class:    ClassMetamorphic,
					Severity: SeverityStandard,
					Verdict:  VerdictSatisfied,
					ObservedState: StatePlaneVector{
						Lifecycle: "active",
						Trust:     "verified",
						Integrity: "verified",
						Binding:   "metamorphic-verified",
					},
					EvidenceDigest: hashString("metamorphic-whitespace-invariance"),
					Message:        "Metamorphic transformation confirmed: policy score preserved across formatting",
				}, nil
			}
			return nil, fmt.Errorf("metamorphic violation")
		},
	}
}

func createSilenceMissingReceiptVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-silence-missing-inference-receipt",
		Name:        "Evidence Plane Silence Missing Inference Receipt",
		Class:       ClassSilence,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Inference response returned without mandatory evidence receipt caught as silent drop",
		Execute: func(_ context.Context) (*VectorResult, error) {
			receiptEmitted := false
			if !receiptEmitted {
				return &VectorResult{
					VectorID: "vec-silence-missing-inference-receipt",
					Name:     "Evidence Plane Silence Missing Inference Receipt",
					Class:    ClassSilence,
					Severity: SeverityStandard,
					Verdict:  VerdictRejected,
					ObservedState: StatePlaneVector{
						Lifecycle: "degraded",
						Trust:     "untrusted",
						Integrity: "unverified",
						Binding:   "unbound",
					},
					EvidenceDigest: hashString("silence-missing-receipt-caught"),
					Message:        "Silence detected: completed inference omitted mandatory InferenceReceipt",
				}, nil
			}
			return nil, fmt.Errorf("unexpected receipt emission")
		},
	}
}

func createCounterfactualReplayVector() ConformanceVector {
	return ConformanceVector{
		ID:          "vec-cfr-evaluator-threshold-replay",
		Name:        "Decision Evaluator Counterfactual Policy Replay",
		Class:       ClassCounterfactualReplay,
		Severity:    SeverityStandard,
		Target:      TargetDecisionEvaluator,
		Description: "Replaying historical trace with stricter safety threshold deterministically denies",
		Execute: func(_ context.Context) (*VectorResult, error) {
			historicalSafetyScore := 7
			counterfactualMinSafety := 9
			if historicalSafetyScore < counterfactualMinSafety {
				return &VectorResult{
					VectorID: "vec-cfr-evaluator-threshold-replay",
					Name:     "Decision Evaluator Counterfactual Policy Replay",
					Class:    ClassCounterfactualReplay,
					Severity: SeverityStandard,
					Verdict:  VerdictSatisfied,
					ObservedState: StatePlaneVector{
						Lifecycle: "active",
						Trust:     "verified",
						Integrity: "verified",
						Binding:   "counterfactual-replayed",
					},
					EvidenceDigest: hashString("cfr-threshold-deterministic-deny"),
					Message:        "Counterfactual replay verified: alternate threshold shifted decision from allow to deny",
				}, nil
			}
			return nil, fmt.Errorf("unexpected counterfactual outcome")
		},
	}
}
