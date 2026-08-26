/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

// EvidenceReceipt is a signed evidence receipt observed by the health tracker.
type EvidenceReceipt struct {
	ID             string
	SignatureValid bool
}

// EvidenceHealthResult is the fail-closed evaluation of an evidence sequence.
type EvidenceHealthResult struct {
	Healthy            bool
	Valid              bool
	ExpectedReceipts   []string
	ObservedReceipts   []string
	MissingReceipts    []string
	UnexpectedReceipts []string
	DuplicateReceipts  []string
	SignatureFailures  []string
	SequenceGaps       []string
}

// EvidenceHealthTracker records receipts against one expected sequence.
type EvidenceHealthTracker struct {
	expected          []string
	observed          []EvidenceReceipt
	signatureFailures []string
}

// NewEvidenceHealthTracker creates a tracker for the supplied receipt order.
func NewEvidenceHealthTracker(expected []string) *EvidenceHealthTracker {
	return &EvidenceHealthTracker{expected: cloneStrings(expected)}
}

// RecordReceipt records one observed receipt for later evaluation.
func (t *EvidenceHealthTracker) RecordReceipt(receipt EvidenceReceipt) {
	if t == nil {
		return
	}
	t.observed = append(t.observed, receipt)
	if receipt.ID == "" || !receipt.SignatureValid {
		t.signatureFailures = append(t.signatureFailures, receipt.ID)
	}
}

// Evaluate returns a deterministic, fail-closed evidence-health result.
func (t *EvidenceHealthTracker) Evaluate() EvidenceHealthResult {
	if t == nil {
		return EvidenceHealthResult{}
	}
	result := EvidenceHealthResult{
		ExpectedReceipts:  cloneStrings(t.expected),
		SignatureFailures: cloneStrings(t.signatureFailures),
	}
	seen := make(map[string]int, len(t.observed))
	for _, receipt := range t.observed {
		result.ObservedReceipts = append(result.ObservedReceipts, receipt.ID)
		seen[receipt.ID]++
		if !contains(t.expected, receipt.ID) && receipt.ID != "" {
			result.UnexpectedReceipts = appendUnique(result.UnexpectedReceipts, receipt.ID)
		}
		if seen[receipt.ID] > 1 && receipt.ID != "" {
			result.DuplicateReceipts = appendUnique(result.DuplicateReceipts, receipt.ID)
		}
	}
	for _, expectedID := range t.expected {
		if seen[expectedID] == 0 {
			result.MissingReceipts = append(result.MissingReceipts, expectedID)
		}
	}
	result.SequenceGaps = findSequenceGaps(t.expected, result.ObservedReceipts)
	result.Valid = len(t.expected) > 0 && len(result.MissingReceipts) == 0 &&
		len(result.UnexpectedReceipts) == 0 && len(result.DuplicateReceipts) == 0 &&
		len(result.SignatureFailures) == 0 && len(result.SequenceGaps) == 0
	result.Healthy = result.Valid
	return result
}

func findSequenceGaps(expected, observed []string) []string {
	observedExpected := make([]string, 0, len(observed))
	for _, receiptID := range observed {
		if contains(expected, receiptID) {
			observedExpected = append(observedExpected, receiptID)
		}
	}

	var gaps []string
	for index, expectedID := range expected {
		if index >= len(observedExpected) || observedExpected[index] != expectedID {
			gaps = append(gaps, expectedID)
		}
	}
	return gaps
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendUnique(values []string, target string) []string {
	if contains(values, target) {
		return values
	}
	return append(values, target)
}
