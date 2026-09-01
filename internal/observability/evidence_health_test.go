/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

const healthTestProducer = "spiffe://ckodex.com/ns/default/sa/evidence-producer"

func healthTestKey(fill byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = fill
	}
	return ed25519.NewKeyFromSeed(seed)
}

func healthDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func healthVerifier(t *testing.T, key ed25519.PrivateKey) *provenance.ReceiptVerifier {
	t.Helper()
	verifier, err := provenance.NewReceiptVerifier(map[string]ed25519.PublicKey{
		healthTestProducer: key.Public().(ed25519.PublicKey),
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return verifier
}

func healthReceipt(t *testing.T, key ed25519.PrivateKey, id string, sequence uint64, previous string) provenance.EvidenceReceipt {
	t.Helper()
	receipt, err := provenance.SignEvidenceReceipt(provenance.EvidenceReceipt{
		ID: id, SubjectDigest: healthDigest("subject-" + id),
		Producer: provenance.ProducerBinding{SPIFFEID: healthTestProducer},
		Sequence: sequence, PreviousDigest: previous,
		ProducedAt: time.Date(2026, 8, 28, 12, int(sequence), 0, 0, time.UTC).Format(time.RFC3339Nano),
	}, key)
	if err != nil {
		t.Fatalf("sign receipt: %v", err)
	}
	return receipt
}

func TestEvidenceHealthSpecToRuntime_VerifiesExpectedSignedChain(t *testing.T) {
	key := healthTestKey(1)
	received := healthReceipt(t, key, "received", 1, "")
	validated := healthReceipt(t, key, "validated", 2, received.Digest)
	signed := healthReceipt(t, key, "signed", 3, validated.Digest)
	tracker := NewEvidenceHealthTracker([]string{"received", "validated", "signed"}, healthVerifier(t, key))
	tracker.RecordReceipt(received)
	tracker.RecordReceipt(validated)
	tracker.RecordReceipt(signed)

	result := tracker.Evaluate()
	if !result.Healthy || !result.Valid {
		t.Fatalf("expected healthy result, got %+v", result)
	}
}

func TestEvidenceHealthRuntimeToSpec_ReportsMissingSignatureAndSequenceFailures(t *testing.T) {
	key := healthTestKey(2)
	received := healthReceipt(t, key, "received", 1, "")
	validated := healthReceipt(t, key, "validated", 2, received.Digest)
	validated.Signature = "invalid"
	tracker := NewEvidenceHealthTracker([]string{"received", "validated", "signed"}, healthVerifier(t, key))
	tracker.RecordReceipt(received)
	tracker.RecordReceipt(validated)

	result := tracker.Evaluate()
	if result.Healthy || result.Valid {
		t.Fatalf("expected unhealthy result, got %+v", result)
	}
	if !reflect.DeepEqual(result.MissingReceipts, []string{"signed"}) {
		t.Fatalf("missing receipts = %v", result.MissingReceipts)
	}
	if !reflect.DeepEqual(result.SequenceGaps, []string{"signed"}) {
		t.Fatalf("sequence gaps = %v", result.SequenceGaps)
	}
	if !reflect.DeepEqual(result.SignatureFailures, []string{"validated"}) {
		t.Fatalf("signature failures = %v", result.SignatureFailures)
	}
}

func TestEvidenceHealthSpecToRuntime_RejectsBrokenProducerAndDigestBindings(t *testing.T) {
	key := healthTestKey(3)
	receipt := healthReceipt(t, key, "received", 1, "")
	receipt.Producer.SPIFFEID = "spiffe://ckodex.com/ns/other/sa/evidence-producer"
	tracker := NewEvidenceHealthTracker([]string{"received"}, healthVerifier(t, key))
	tracker.RecordReceipt(receipt)
	result := tracker.Evaluate()
	if !reflect.DeepEqual(result.ProducerBindingFailures, []string{"received"}) {
		t.Fatalf("producer failures = %v", result.ProducerBindingFailures)
	}

	tampered := healthReceipt(t, key, "received", 1, "")
	tampered.SubjectDigest = healthDigest("tampered")
	tracker = NewEvidenceHealthTracker([]string{"received"}, healthVerifier(t, key))
	tracker.RecordReceipt(tampered)
	result = tracker.Evaluate()
	if !reflect.DeepEqual(result.IntegrityFailures, []string{"received"}) {
		t.Fatalf("integrity failures = %v", result.IntegrityFailures)
	}
}

func TestEvidenceHealthRuntimeToSpec_ReportsUnexpectedDuplicateAndBrokenChain(t *testing.T) {
	key := healthTestKey(4)
	received := healthReceipt(t, key, "received", 1, "")
	duplicate := healthReceipt(t, key, "received", 2, received.Digest)
	foreign := healthReceipt(t, key, "foreign", 3, healthDigest("wrong predecessor"))
	tracker := NewEvidenceHealthTracker([]string{"received"}, healthVerifier(t, key))
	tracker.RecordReceipt(received)
	tracker.RecordReceipt(duplicate)
	tracker.RecordReceipt(foreign)

	result := tracker.Evaluate()
	if result.Healthy || result.Valid {
		t.Fatalf("expected unhealthy result, got %+v", result)
	}
	if !reflect.DeepEqual(result.DuplicateReceipts, []string{"received"}) {
		t.Fatalf("duplicates = %v", result.DuplicateReceipts)
	}
	if !reflect.DeepEqual(result.UnexpectedReceipts, []string{"foreign"}) {
		t.Fatalf("unexpected = %v", result.UnexpectedReceipts)
	}
	if !reflect.DeepEqual(result.SequenceGaps, []string{"received", "foreign"}) {
		t.Fatalf("sequence gaps = %v", result.SequenceGaps)
	}
}

func TestEvidenceHealthTrackerFailsClosedWithoutExpectationOrVerifier(t *testing.T) {
	if result := NewEvidenceHealthTracker(nil, nil).Evaluate(); result.Healthy || result.Valid {
		t.Fatalf("empty tracker must fail closed: %+v", result)
	}
	var tracker *EvidenceHealthTracker
	tracker.RecordReceipt(provenance.EvidenceReceipt{})
	if result := tracker.Evaluate(); result.Healthy || result.Valid {
		t.Fatalf("nil tracker must fail closed: %+v", result)
	}
}

func TestEvidenceHealthMonitorRuntimeToSpec_ExposesStickyFailure(t *testing.T) {
	monitor := &EvidenceHealthMonitor{}
	if err := monitor.Check(nil); err != nil {
		t.Fatalf("new monitor check: %v", err)
	}
	monitor.RecordFailure(errors.New("signature verification failed"))
	if err := monitor.Check(nil); err == nil || err.Error() != "signature verification failed" {
		t.Fatalf("monitor error = %v", err)
	}
	monitor.Observe(EvidenceHealthResult{Healthy: true, Valid: true})
	if err := monitor.Check(nil); err == nil {
		t.Fatal("successful observation must not erase prior evidence failure")
	}
}
