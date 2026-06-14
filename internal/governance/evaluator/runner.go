/*
Copyright 2026 CKodex Authors.
*/

package evaluator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InternalReceiptScheme is the URI scheme used for evaluation receipts generated
// by this runner. Receipts under this scheme are asserted-only — they are not
// backed by an external cryptographic verifier — so HasVerifiedSupplyChainEvidence
// (in the governance package) rejects them.
const InternalReceiptScheme = "ckodex://"

// internalReceiptPrefix is the full prefix for receipts produced by this runner.
const internalReceiptPrefix = InternalReceiptScheme + "eval-runner/reports/"

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

	// Evaluation receipts are asserted-only until an external verifier publishes
	// a registry-backed attestation. We still hash the actual report payload so
	// the receipt is deterministic and reviewable.
	payload, err := json.Marshal(report)
	if err == nil {
		sum := sha256.Sum256(payload)
		adapter.Status.EvidenceBundle.SignatureDigest = "sha256:" + hex.EncodeToString(sum[:])
	} else {
		adapter.Status.EvidenceBundle.SignatureDigest = fmt.Sprintf("sha256:%x", report.VerificationTime.UnixNano())
	}
	adapter.Status.EvidenceBundle.AttestationURI = internalReceiptPrefix + adapter.Name
}
