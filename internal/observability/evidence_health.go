/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

// EvidenceHealthResult is the fail-closed evaluation of an evidence sequence.
type EvidenceHealthResult struct {
	Healthy                 bool
	Valid                   bool
	ExpectedReceipts        []string
	ObservedReceipts        []string
	MissingReceipts         []string
	UnexpectedReceipts      []string
	DuplicateReceipts       []string
	SignatureFailures       []string
	ProducerBindingFailures []string
	IntegrityFailures       []string
	SequenceGaps            []string
}

// EvidenceHealthTracker verifies receipts against one expected sequence.
type EvidenceHealthTracker struct {
	expected []string
	observed []provenance.EvidenceReceipt
	verifier *provenance.ReceiptVerifier
}

// NewEvidenceHealthTracker creates a tracker for the supplied receipt order
// and producer trust configuration.
func NewEvidenceHealthTracker(expected []string, verifier *provenance.ReceiptVerifier) *EvidenceHealthTracker {
	return &EvidenceHealthTracker{expected: cloneStrings(expected), verifier: verifier}
}

// RecordReceipt records one observed receipt for later verification.
func (t *EvidenceHealthTracker) RecordReceipt(receipt provenance.EvidenceReceipt) {
	if t != nil {
		t.observed = append(t.observed, receipt)
	}
}

// Evaluate returns a deterministic, fail-closed evidence-health result.
func (t *EvidenceHealthTracker) Evaluate() EvidenceHealthResult {
	if t == nil {
		return EvidenceHealthResult{}
	}
	result := EvidenceHealthResult{ExpectedReceipts: cloneStrings(t.expected)}
	seen := make(map[string]int, len(t.observed))
	for _, receipt := range t.observed {
		result.ObservedReceipts = append(result.ObservedReceipts, receipt.ID)
		seen[receipt.ID]++
		classifyReceiptVerification(&result, receipt.ID, t.verifier, receipt)
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
	result.SequenceGaps = findReceiptSequenceGaps(t.expected, t.observed)
	result.Valid = len(t.expected) > 0 && t.verifier != nil && result.failureCount() == 0
	result.Healthy = result.Valid
	return result
}

func classifyReceiptVerification(result *EvidenceHealthResult, receiptID string, verifier *provenance.ReceiptVerifier, receipt provenance.EvidenceReceipt) {
	err := verifier.Verify(receipt)
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, provenance.ErrReceiptSignature):
		result.SignatureFailures = appendUnique(result.SignatureFailures, receiptID)
	case errors.Is(err, provenance.ErrReceiptProducer):
		result.ProducerBindingFailures = appendUnique(result.ProducerBindingFailures, receiptID)
	default:
		result.IntegrityFailures = appendUnique(result.IntegrityFailures, receiptID)
	}
}

func (r EvidenceHealthResult) failureCount() int {
	return len(r.MissingReceipts) + len(r.UnexpectedReceipts) + len(r.DuplicateReceipts) +
		len(r.SignatureFailures) + len(r.ProducerBindingFailures) + len(r.IntegrityFailures) + len(r.SequenceGaps)
}

func findReceiptSequenceGaps(expected []string, observed []provenance.EvidenceReceipt) []string {
	var gaps []string
	for index, receipt := range observed {
		if index >= len(expected) || receipt.ID != expected[index] || receipt.Sequence != uint64(index+1) {
			gaps = appendUnique(gaps, receipt.ID)
		}
		if index == 0 {
			if receipt.PreviousDigest != "" {
				gaps = appendUnique(gaps, receipt.ID)
			}
			continue
		}
		if receipt.PreviousDigest != observed[index-1].Digest {
			gaps = appendUnique(gaps, receipt.ID)
		}
	}
	for index := len(observed); index < len(expected); index++ {
		gaps = appendUnique(gaps, expected[index])
	}
	return gaps
}

// EvidenceHealthMonitor makes verified receipt failures visible through a
// manager health check. Failures are sticky for the process lifetime.
type EvidenceHealthMonitor struct {
	mu      sync.RWMutex
	failure error
}

func (m *EvidenceHealthMonitor) Observe(result EvidenceHealthResult) {
	if result.Healthy {
		return
	}
	failures := result.failureCount()
	if failures == 0 {
		failures = 1
	}
	m.RecordFailure(fmt.Errorf("evidence receipt verification failed (%d failures)", failures))
}

func (m *EvidenceHealthMonitor) RecordFailure(err error) {
	if m == nil || err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure == nil {
		m.failure = err
	}
}

func (m *EvidenceHealthMonitor) Check(_ *http.Request) error {
	if m == nil {
		return errors.New("evidence health monitor is unavailable")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failure
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
