/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package channels

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func testDigest(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func testContext(cid string, seq uint64) CorrelationContext {
	return CorrelationContext{
		CorrelationID: cid,
		WorkloadID:    "workload-test",
		TenantID:      "tenant-alpha",
		Timestamp:     time.Now().UTC(),
		Sequence:      seq,
	}
}

func populateNominalRecords(t *testing.T, fr *FlightRecorder, cid string) {
	t.Helper()
	tRec := TelemetryRecord{
		CorrelationContext: testContext(cid, 1),
		MetricName:         "infer.latency_ms",
		MetricValue:        42.5,
		Unit:               "ms",
		State:              TelemetryStateNominal,
		Labels:             map[string]string{"env": "prod"},
	}
	if err := fr.RecordTelemetry(tRec); err != nil {
		t.Fatalf("record telemetry failed: %v", err)
	}
	dRec := DecisionRecord{
		CorrelationContext:  testContext(cid, 2),
		PolicyID:            "pol-governance-v1",
		RuleID:              "rule-admit-approved",
		Disposition:         DispositionPass,
		EvaluatorAppraisal:  "appraisal:score=0.98",
		FactsDigest:         testDigest("facts-snapshot-001"),
		EvaluationLatencyMs: 12,
		ReasonCode:          "RC_APPROVED",
	}
	if err := fr.RecordDecision(dRec); err != nil {
		t.Fatalf("record decision failed: %v", err)
	}
}

func populateNominalExecutionAndEvidence(t *testing.T, fr *FlightRecorder, cid string) {
	t.Helper()
	eRec := ExecutionRecord{
		CorrelationContext: testContext(cid, 3),
		Component:          "vllm-engine",
		Action:             "inference_start",
		FromState:          ExecutionStatePending,
		ToState:            ExecutionStateRunning,
		ResourceRef:        "pod/vllm-worker-0",
		State:              ExecutionStateRunning,
	}
	if err := fr.RecordExecution(eRec); err != nil {
		t.Fatalf("record execution failed: %v", err)
	}
	evRec := EvidenceRecord{
		CorrelationContext: testContext(cid, 4),
		ReceiptID:          "rcpt-001",
		SubjectDigest:      testDigest("receipt-subject-001"),
		ProducerSPIFFE:     "spiffe://ckodex.local/ns/default/sa/operator",
		ProducerKeyDigest:  testDigest("key-001"),
		State:              EvidenceStateVerified,
		SignatureRef:       "sig-ref-001",
	}
	if err := fr.RecordEvidence(evRec); err != nil {
		t.Fatalf("record evidence failed: %v", err)
	}
}

func TestFourTruthChannels_NominalFlow(t *testing.T) {
	fr, err := NewFlightRecorder(10, "producer-node-1")
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
	cid := "corr-nominal-001"
	populateNominalRecords(t, fr, cid)
	populateNominalExecutionAndEvidence(t, fr, cid)

	traces, err := fr.Correlate(cid)
	if err != nil {
		t.Fatalf("correlate traces failed: %v", err)
	}
	if traces.State != InvariantStateSatisfied {
		t.Fatalf("expected InvariantStateSatisfied, got %s (violations: %v)", traces.State, traces.Violations)
	}
	if len(traces.Telemetry) != 1 || len(traces.Decision) != 1 || len(traces.Execution) != 1 || len(traces.Evidence) != 1 {
		t.Fatalf("unexpected trace lengths in flight correlation")
	}
}

func TestFlightRecorder_AntiExecuteViolation(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-anti-exec"
	now := time.Now().UTC()

	dRec := DecisionRecord{
		CorrelationContext: CorrelationContext{CorrelationID: cid, WorkloadID: "w", Timestamp: now, Sequence: 1},
		PolicyID:           "pol-deny-all",
		RuleID:             "rule-deny",
		Disposition:        DispositionDeny,
		FactsDigest:        testDigest("facts-payload"),
	}
	_ = fr.RecordDecision(dRec)

	eRec := ExecutionRecord{
		CorrelationContext: CorrelationContext{CorrelationID: cid, WorkloadID: "w", Timestamp: now.Add(time.Second), Sequence: 2},
		Component:          "engine-bad",
		State:              ExecutionStateRunning,
	}
	_ = fr.RecordExecution(eRec)

	state, violations, err := fr.VerifyInvariants(cid)
	if err != nil {
		t.Fatalf("verify invariants error: %v", err)
	}
	if state != InvariantStateViolated {
		t.Fatalf("expected InvariantStateViolated, got %s", state)
	}
	if len(violations) == 0 || !strings.Contains(violations[0], "anti_execute") {
		t.Fatalf("expected anti_execute violation, got %v", violations)
	}
}

func TestFlightRecorder_ActiveUntrustedViolation(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-active-untrusted"

	evRec := EvidenceRecord{
		CorrelationContext: testContext(cid, 1),
		ReceiptID:          "rcpt-rejected",
		SubjectDigest:      testDigest("subj"),
		ProducerKeyDigest:  testDigest("key"),
		State:              EvidenceStateRejected,
	}
	_ = fr.RecordEvidence(evRec)

	eRec := ExecutionRecord{
		CorrelationContext: testContext(cid, 2),
		Component:          "worker",
		State:              ExecutionStateRunning,
	}
	_ = fr.RecordExecution(eRec)

	state, violations, err := fr.VerifyInvariants(cid)
	if err != nil {
		t.Fatalf("verify invariants error: %v", err)
	}
	if state != InvariantStateViolated {
		t.Fatalf("expected InvariantStateViolated, got %s", state)
	}
	if len(violations) == 0 || !strings.Contains(violations[0], "active_untrusted") {
		t.Fatalf("expected active_untrusted violation, got %v", violations)
	}
}

func TestFlightRecorder_DecisionMissingViolation(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-decision-missing"

	eRec := ExecutionRecord{
		CorrelationContext: testContext(cid, 1),
		Component:          "worker",
		State:              ExecutionStateCompleted,
	}
	_ = fr.RecordExecution(eRec)

	state, violations, err := fr.VerifyInvariants(cid)
	if err != nil {
		t.Fatalf("verify invariants error: %v", err)
	}
	if state != InvariantStateViolated {
		t.Fatalf("expected InvariantStateViolated, got %s", state)
	}
	if len(violations) == 0 || !strings.Contains(violations[0], "decision_missing") {
		t.Fatalf("expected decision_missing violation, got %v", violations)
	}
}

func TestFlightRecorder_EvidenceChainBreak(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-chain-break"

	ev1 := EvidenceRecord{
		CorrelationContext: testContext(cid, 1),
		ReceiptID:          "rcpt-1",
		SubjectDigest:      testDigest("subj-1"),
		ProducerKeyDigest:  testDigest("key-1"),
		PreviousDigest:     testDigest("unlinked-root"),
	}
	_ = fr.RecordEvidence(ev1)

	state, violations, err := fr.VerifyInvariants(cid)
	if err != nil {
		t.Fatalf("verify invariants error: %v", err)
	}
	if state != InvariantStateViolated || !strings.Contains(violations[0], "chain_break") {
		t.Fatalf("expected chain_break on root previous digest, got %s, %v", state, violations)
	}
}

func TestConfidentiality_PIIAndSecretRedaction(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-pii-scrub"

	rawSecret := "User SSN 123-45-6789 and Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9 and sk-1234567890abcdef123456"
	tRec := TelemetryRecord{
		CorrelationContext: testContext(cid, 1),
		MetricName:         "metric-safe",
		MetricValue:        1.0,
		State:              TelemetryStateNominal,
		Labels: map[string]string{
			"leak_test": rawSecret,
			"user_mail": "alice@example.com",
			"ip_addr":   "192.168.1.100",
		},
	}
	if err := fr.RecordTelemetry(tRec); err != nil {
		t.Fatalf("record telemetry failed: %v", err)
	}

	traces, err := fr.Correlate(cid)
	if err != nil {
		t.Fatalf("correlate failed: %v", err)
	}
	labels := traces.Telemetry[0].Labels
	for k, v := range labels {
		if strings.Contains(v, "123-45-6789") || strings.Contains(v, "alice@example.com") || strings.Contains(v, "192.168.1.100") {
			t.Fatalf("PII leaked in label %s: %s", k, v)
		}
		if strings.Contains(v, "Bearer") || strings.Contains(v, "sk-1234567890") {
			t.Fatalf("Secret token leaked in label %s: %s", k, v)
		}
	}
}

func TestValidation_ContextAndDigestValidation(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")

	invalidContext := CorrelationContext{CorrelationID: "", Sequence: 1}
	if err := fr.RecordTelemetry(TelemetryRecord{CorrelationContext: invalidContext}); err == nil {
		t.Fatalf("expected error on empty correlation ID")
	}

	zeroSeq := CorrelationContext{CorrelationID: "cid", Sequence: 0}
	if err := fr.RecordExecution(ExecutionRecord{CorrelationContext: zeroSeq}); err == nil {
		t.Fatalf("expected error on zero sequence")
	}

	validCtx := testContext("cid-digest-test", 1)
	badDigest := DecisionRecord{
		CorrelationContext: validCtx,
		FactsDigest:        "invalid-not-sha256",
	}
	if err := fr.RecordDecision(badDigest); err == nil {
		t.Fatalf("expected error on non-sha256 digest")
	}
}

func TestFlightRecorder_SealingAndImmutability(t *testing.T) {
	fr, _ := NewFlightRecorder(10, "producer-node-1")
	cid := "corr-seal-001"

	_ = fr.RecordTelemetry(TelemetryRecord{CorrelationContext: testContext(cid, 1), MetricName: "cpu", MetricValue: 12.0})
	_ = fr.RecordDecision(DecisionRecord{CorrelationContext: testContext(cid, 2), FactsDigest: testDigest("facts")})

	sealed, err := fr.Seal(cid)
	if err != nil {
		t.Fatalf("seal failed: %v", err)
	}
	if !strings.HasPrefix(sealed.RootDigest, "sha256:") {
		t.Fatalf("invalid root digest format: %s", sealed.RootDigest)
	}
	if sealed.RecordCount != 2 {
		t.Fatalf("expected record count 2, got %d", sealed.RecordCount)
	}

	err = fr.RecordTelemetry(TelemetryRecord{CorrelationContext: testContext(cid, 3), MetricName: "mem", MetricValue: 40.0})
	if err == nil {
		t.Fatalf("expected ErrRecorderSealed on appending to sealed correlation")
	}

	sealed2, err := fr.Seal(cid)
	if err != nil || sealed2.RootDigest != sealed.RootDigest {
		t.Fatalf("seal must be idempotent and preserve root digest")
	}
}

func TestFlightRecorder_BoundedCapacityEviction(t *testing.T) {
	fr, _ := NewFlightRecorder(2, "producer-node-1")

	_ = fr.RecordTelemetry(TelemetryRecord{CorrelationContext: testContext("cid-1", 1), MetricName: "m1"})
	_ = fr.RecordTelemetry(TelemetryRecord{CorrelationContext: testContext("cid-2", 1), MetricName: "m2"})
	_ = fr.RecordTelemetry(TelemetryRecord{CorrelationContext: testContext("cid-3", 1), MetricName: "m3"})

	if _, err := fr.Correlate("cid-1"); err == nil {
		t.Fatalf("expected cid-1 to be evicted under capacity limit")
	}
	if _, err := fr.Correlate("cid-2"); err != nil {
		t.Fatalf("expected cid-2 to exist: %v", err)
	}
	if _, err := fr.Correlate("cid-3"); err != nil {
		t.Fatalf("expected cid-3 to exist: %v", err)
	}
}

func TestFlightRecorder_ConcurrentAccess(t *testing.T) {
	fr, _ := NewFlightRecorder(100, "producer-node-1")
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cid := fmt.Sprintf("corr-conc-%d", idx%5)
			_ = fr.RecordTelemetry(TelemetryRecord{
				CorrelationContext: testContext(cid, uint64(idx+1)),
				MetricName:         "m_concurrent",
				MetricValue:        float64(idx),
			})
			_, _ = fr.Correlate(cid)
		}(i)
	}
	wg.Wait()
}
