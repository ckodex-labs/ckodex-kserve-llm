/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"reflect"
	"testing"
)

func TestEvidenceHealthTrackerHealthySequence(t *testing.T) {
	tracker := NewEvidenceHealthTracker([]string{"received", "validated", "signed"})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "validated", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "signed", SignatureValid: true})

	result := tracker.Evaluate()
	if !result.Healthy || !result.Valid {
		t.Fatalf("expected healthy result, got %+v", result)
	}
}

func TestEvidenceHealthTrackerFailsClosedForMissingAndInvalidEvidence(t *testing.T) {
	tracker := NewEvidenceHealthTracker([]string{"received", "validated", "signed"})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "validated", SignatureValid: false})

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

func TestEvidenceHealthTrackerDetectsUnexpectedDuplicateAndEmptyReceipts(t *testing.T) {
	tracker := NewEvidenceHealthTracker([]string{"received"})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "foreign", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{SignatureValid: false})

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
	if !reflect.DeepEqual(result.SignatureFailures, []string{""}) {
		t.Fatalf("signature failures = %v", result.SignatureFailures)
	}
}

func TestEvidenceHealthTrackerRejectsEmptyExpectedSequence(t *testing.T) {
	result := NewEvidenceHealthTracker(nil).Evaluate()
	if result.Healthy || result.Valid {
		t.Fatalf("expected empty sequence to fail closed, got %+v", result)
	}
}

func TestEvidenceHealthTrackerRejectsOutOfOrderReceipts(t *testing.T) {
	tracker := NewEvidenceHealthTracker([]string{"received", "validated", "signed"})
	tracker.RecordReceipt(EvidenceReceipt{ID: "validated", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "received", SignatureValid: true})
	tracker.RecordReceipt(EvidenceReceipt{ID: "signed", SignatureValid: true})

	result := tracker.Evaluate()
	if result.Healthy || result.Valid {
		t.Fatalf("expected out-of-order sequence to fail closed, got %+v", result)
	}
	if !reflect.DeepEqual(result.SequenceGaps, []string{"received", "validated"}) {
		t.Fatalf("sequence gaps = %v", result.SequenceGaps)
	}
}

func TestEvidenceHealthTrackerNilReceiverFailsClosed(t *testing.T) {
	var tracker *EvidenceHealthTracker
	tracker.RecordReceipt(EvidenceReceipt{ID: "ignored", SignatureValid: true})
	result := tracker.Evaluate()
	if result.Healthy || result.Valid {
		t.Fatalf("expected nil tracker to fail closed, got %+v", result)
	}
}

func TestNewEvidenceHealthTrackerCopiesExpectedSequence(t *testing.T) {
	expected := []string{"received"}
	tracker := NewEvidenceHealthTracker(expected)
	expected[0] = "changed"
	result := tracker.Evaluate()
	if !reflect.DeepEqual(result.ExpectedReceipts, []string{"received"}) {
		t.Fatalf("expected sequence was not copied: %v", result.ExpectedReceipts)
	}
}
