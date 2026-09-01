/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

type receiptCommitmentInput struct {
	ExecID  string            `json:"execId"`
	Status  string            `json:"status"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details"`
}

// LogReceipt preserves the existing controller call contract while emitting
// only a content commitment. It intentionally reports the result as unverified:
// no signing key or producer trust binding is available on this legacy path.
func (a *AuditLogger) LogReceipt(ctx context.Context, execID, status, summary string, details map[string]string) {
	commitment := receiptInputCommitment(execID, status, summary, details)
	a.emit(ctx, AuditEvent{
		Action: "ReceiptCommitment", Resource: commitment, Actor: "ckodex-operator",
		Outcome: AuditSuccess, Timestamp: time.Now(), ExecID: execID,
		ExecKind: SignalReceipt, ReproducibilityClass: ReproBounded,
		Reason: "cryptographic receipt unavailable",
		Details: map[string]string{
			"evidence.commitment":       commitment,
			"evidence.integrity":        "unverified",
			"evidence.producer_binding": "unverified",
			"evidence.signature":        "absent",
		},
	})
}

// LogVerifiedReceiptSequence verifies an entire expected chain before emitting
// digest-only receipt events. Verification does not imply Rekor inclusion.
func (a *AuditLogger) LogVerifiedReceiptSequence(
	ctx context.Context,
	expected []string,
	receipts []provenance.EvidenceReceipt,
	verifier *provenance.ReceiptVerifier,
) error {
	tracker := NewEvidenceHealthTracker(expected, verifier)
	for _, receipt := range receipts {
		tracker.RecordReceipt(receipt)
	}
	result := tracker.Evaluate()
	a.evidenceHealth.Observe(result)
	if !result.Healthy {
		a.emitReceiptVerificationFailure(ctx, result)
		return fmt.Errorf("verify evidence receipt sequence: %d invariant failures", result.failureCount())
	}
	for _, receipt := range receipts {
		a.emitVerifiedReceipt(ctx, receipt)
	}
	return nil
}

// EvidenceHealthCheck exposes sticky receipt-verification failures to the
// controller-runtime readiness endpoint.
func (a *AuditLogger) EvidenceHealthCheck(request *http.Request) error {
	if a == nil || a.evidenceHealth == nil {
		return fmt.Errorf("evidence health monitor is unavailable")
	}
	return a.evidenceHealth.Check(request)
}

func (a *AuditLogger) emitReceiptVerificationFailure(ctx context.Context, result EvidenceHealthResult) {
	a.emit(ctx, AuditEvent{
		Action: "ReceiptVerification", Resource: "evidence/receipt-sequence",
		Actor: "ckodex-operator", Outcome: AuditFailure, Timestamp: time.Now(),
		ExecKind: SignalReceipt, ReproducibilityClass: ReproBounded,
		Reason: "evidence receipt verification failed",
		Details: map[string]string{
			"evidence.failure_count": fmt.Sprintf("%d", result.failureCount()),
			"evidence.integrity":     "invalid",
		},
	})
}

func (a *AuditLogger) emitVerifiedReceipt(ctx context.Context, receipt provenance.EvidenceReceipt) {
	a.emit(ctx, AuditEvent{
		Action: "Receipt", Resource: receipt.SubjectDigest,
		Actor: receipt.Producer.SPIFFEID, Outcome: AuditSuccess, Timestamp: time.Now(),
		ExecID: receipt.ID, ExecKind: SignalReceipt, ReproducibilityClass: ReproBounded,
		Details: map[string]string{
			"evidence.digest":       receipt.Digest,
			"evidence.key_digest":   receipt.Producer.KeyDigest,
			"evidence.sequence":     fmt.Sprintf("%d", receipt.Sequence),
			"evidence.signature":    "verified",
			"evidence.transparency": "unverified",
		},
	})
}

func receiptInputCommitment(execID, status, summary string, details map[string]string) string {
	encoded, err := json.Marshal(receiptCommitmentInput{ExecID: execID, Status: status, Summary: summary, Details: details})
	if err != nil {
		encoded = []byte("receipt-commitment-encoding-failed")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}
