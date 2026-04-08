/*
Copyright 2026 CKodex Authors.
*/

package evaluator

import (
	"fmt"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvalReport represents the results of an automated evaluation run.
type EvalReport struct {
	SafetyScore      float64   `json:"safetyScore"`
	RefusalRate      float64   `json:"refusalRate"`
	CompatibilityV   []float64 `json:"compatibilityVector"`
	VerificationTime time.Time `json:"verificationTime"`
}

// GenerateEvidence populates the EvidenceBundle and BehaviorMetadata based on an eval report.
func GenerateEvidence(adapter *servingv1alpha2.LLMLoraAdapter, report *EvalReport) {
	if adapter.Spec.Behavior == nil {
		adapter.Spec.Behavior = &servingv1alpha2.BehaviorMetadata{}
	}
	
	// Update Behavior Metadata
	adapter.Spec.Behavior.Safety = int(report.SafetyScore)
	
	// Update Evidence Bundle
	now := metav1.NewTime(report.VerificationTime)
	adapter.Status.EvidenceBundle.LastVerifiedAt = &now
	
	// Mock signature of the report
	adapter.Status.EvidenceBundle.SignatureDigest = fmt.Sprintf("sha256:%x", report.VerificationTime.UnixNano())
	adapter.Status.EvidenceBundle.AttestationURI = "ckodex://eval-runner/reports/" + adapter.Name
}
