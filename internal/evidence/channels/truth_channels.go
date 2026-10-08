/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package channels implements the Four Truth Channels (Telemetry,
// Execution, Decision, Evidence) and the associated Flight Recorder.
package channels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ChannelType identifies each of the Four Truth Channels.
type ChannelType string

const (
	ChannelTelemetry ChannelType = "telemetry"
	ChannelExecution ChannelType = "execution"
	ChannelDecision  ChannelType = "decision"
	ChannelEvidence  ChannelType = "evidence"
)

// Vector states for truth channels (Constitution §13 / §10: vector state, not booleans).
type TelemetryState string

const (
	TelemetryStateNominal   TelemetryState = "nominal"
	TelemetryStateDegraded  TelemetryState = "degraded"
	TelemetryStateAnomalous TelemetryState = "anomalous"
	TelemetryStateCritical  TelemetryState = "critical"
)

type ExecutionState string

const (
	ExecutionStatePending   ExecutionState = "pending"
	ExecutionStateRunning   ExecutionState = "running"
	ExecutionStateCompleted ExecutionState = "completed"
	ExecutionStateFailed    ExecutionState = "failed"
	ExecutionStateHalted    ExecutionState = "halted"
)

type DecisionDisposition string

const (
	DispositionPass       DecisionDisposition = "PASS"
	DispositionDegrade    DecisionDisposition = "DEGRADE"
	DispositionDeny       DecisionDisposition = "DENY"
	DispositionQuarantine DecisionDisposition = "QUARANTINE"
	DispositionEscalate   DecisionDisposition = "ESCALATE"
)

type EvidenceState string

const (
	EvidenceStateCommitted  EvidenceState = "committed"
	EvidenceStateVerified   EvidenceState = "verified"
	EvidenceStateRejected   EvidenceState = "rejected"
	EvidenceStateQuarantine EvidenceState = "quarantine"
)

type InvariantState string

const (
	InvariantStateSatisfied     InvariantState = "satisfied"
	InvariantStateViolated      InvariantState = "violated"
	InvariantStateIndeterminate InvariantState = "indeterminate"
)

type RecorderState string

const (
	RecorderStateOperational RecorderState = "operational"
	RecorderStateDegraded    RecorderState = "degraded"
	RecorderStateSealed      RecorderState = "sealed"
)

var (
	ErrEmptyCorrelation    = errors.New("correlation ID is required")
	ErrZeroSequence        = errors.New("sequence must be greater than zero")
	ErrInvalidDigestFormat = errors.New("digest must be 64-hex characters with sha256: prefix")
	ErrRecorderCapacity    = errors.New("flight recorder capacity must be positive")
	ErrRecorderSealed      = errors.New("flight recorder is sealed for this correlation")
	ErrCorrelationNotFound = errors.New("correlation ID not found in flight recorder")
)

var (
	piiSSN         = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	piiCreditCard  = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)
	piiEmail       = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	piiIPv4        = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	secretBearer   = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`)
	secretKeyBlock = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	secretAPIKey   = regexp.MustCompile(`(?i)(?:api_key|sk-[a-zA-Z0-9]{20,})`)
)

const RedactedMask = "__REDACTED__"

// SanitizeText scrubs known PII, credentials, and token structures from text.
func SanitizeText(s string) string {
	if s == "" {
		return s
	}
	out := piiSSN.ReplaceAllString(s, RedactedMask)
	out = piiCreditCard.ReplaceAllString(out, RedactedMask)
	out = piiEmail.ReplaceAllString(out, RedactedMask)
	out = piiIPv4.ReplaceAllString(out, RedactedMask)
	out = secretBearer.ReplaceAllString(out, RedactedMask)
	out = secretKeyBlock.ReplaceAllString(out, RedactedMask)
	return secretAPIKey.ReplaceAllString(out, RedactedMask)
}

// SanitizeLabels scrubs map values while keeping keys intact.
func SanitizeLabels(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = SanitizeText(v)
	}
	return out
}

// ValidateSHA256 ensures a string conforms to sha256:<64 lowercase hex>.
func ValidateSHA256(v string) error {
	if !strings.HasPrefix(v, "sha256:") || len(v) != 7+sha256.Size*2 {
		return fmt.Errorf("%w: %q", ErrInvalidDigestFormat, v)
	}
	raw := strings.TrimPrefix(v, "sha256:")
	if strings.ToLower(raw) != raw {
		return fmt.Errorf("%w: lowercase required for %q", ErrInvalidDigestFormat, v)
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return fmt.Errorf("%w: hex decode failed: %w", ErrInvalidDigestFormat, err)
	}
	return nil
}

// CorrelationContext anchors entries across all four channels.
type CorrelationContext struct {
	CorrelationID string    `json:"correlationId"`
	WorkloadID    string    `json:"workloadId"`
	TenantID      string    `json:"tenantId,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Sequence      uint64    `json:"sequence"`
}

func validateContext(c CorrelationContext) error {
	if strings.TrimSpace(c.CorrelationID) == "" {
		return fmt.Errorf("%w: empty context", ErrEmptyCorrelation)
	}
	if c.Sequence == 0 {
		return fmt.Errorf("%w: context sequence %d", ErrZeroSequence, c.Sequence)
	}
	return nil
}

// TelemetryRecord contains operational metrics in Channel 1.
type TelemetryRecord struct {
	CorrelationContext
	MetricName  string            `json:"metricName"`
	MetricValue float64           `json:"metricValue"`
	Unit        string            `json:"unit"`
	State       TelemetryState    `json:"state"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// ExecutionRecord captures runtime orchestration progression in Channel 2.
type ExecutionRecord struct {
	CorrelationContext
	Component   string         `json:"component"`
	Action      string         `json:"action"`
	FromState   ExecutionState `json:"fromState"`
	ToState     ExecutionState `json:"toState"`
	ResourceRef string         `json:"resourceRef"`
	State       ExecutionState `json:"state"`
}

// DecisionRecord records evaluations and appraisals in Channel 3 without CoT.
type DecisionRecord struct {
	CorrelationContext
	PolicyID            string              `json:"policyId"`
	RuleID              string              `json:"ruleId"`
	Disposition         DecisionDisposition `json:"disposition"`
	EvaluatorAppraisal  string              `json:"evaluatorAppraisal"`
	FactsDigest         string              `json:"factsDigest"`
	EvaluationLatencyMs int64               `json:"evaluationLatencyMs"`
	ReasonCode          string              `json:"reasonCode"`
}

// EvidenceRecord represents verifiable cryptographic attestations in Channel 4.
type EvidenceRecord struct {
	CorrelationContext
	ReceiptID         string        `json:"receiptId"`
	SubjectDigest     string        `json:"subjectDigest"`
	ProducerSPIFFE    string        `json:"producerSpiffe"`
	ProducerKeyDigest string        `json:"producerKeyDigest"`
	State             EvidenceState `json:"state"`
	PreviousDigest    string        `json:"previousDigest,omitempty"`
	SignatureRef      string        `json:"signatureRef"`
}

// CorrelatedFlightTraces contains the full 4-channel view for a correlation.
type CorrelatedFlightTraces struct {
	CorrelationID string            `json:"correlationId"`
	Telemetry     []TelemetryRecord `json:"telemetry"`
	Execution     []ExecutionRecord `json:"execution"`
	Decision      []DecisionRecord  `json:"decision"`
	Evidence      []EvidenceRecord  `json:"evidence"`
	State         InvariantState    `json:"state"`
	Violations    []string          `json:"violations,omitempty"`
}

// SealedFlightRecord represents a cryptographically committed flight record.
type SealedFlightRecord struct {
	CorrelationID string         `json:"correlationId"`
	RootDigest    string         `json:"rootDigest"`
	RecordCount   int            `json:"recordCount"`
	State         InvariantState `json:"state"`
	SealedAt      time.Time      `json:"sealedAt"`
	ProducerID    string         `json:"producerId"`
}

// FlightRecorder buffers and correlates events across all Four Truth Channels.
type FlightRecorder struct {
	mu         sync.RWMutex
	capacity   int
	producerID string
	state      RecorderState
	order      []string
	seen       map[string]struct{}
	telemetry  map[string][]TelemetryRecord
	execution  map[string][]ExecutionRecord
	decision   map[string][]DecisionRecord
	evidence   map[string][]EvidenceRecord
	sealed     map[string]*SealedFlightRecord
}

// NewFlightRecorder initializes a bounded, thread-safe flight recorder.
func NewFlightRecorder(capacity int, producerID string) (*FlightRecorder, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: capacity %d", ErrRecorderCapacity, capacity)
	}
	return &FlightRecorder{
		capacity: capacity, producerID: producerID, state: RecorderStateOperational,
		seen: make(map[string]struct{}), telemetry: make(map[string][]TelemetryRecord),
		execution: make(map[string][]ExecutionRecord), decision: make(map[string][]DecisionRecord),
		evidence: make(map[string][]EvidenceRecord), sealed: make(map[string]*SealedFlightRecord),
	}, nil
}

// RecordTelemetry records an entry to Channel 1 (Telemetry).
func (fr *FlightRecorder) RecordTelemetry(r TelemetryRecord) error {
	if err := validateContext(r.CorrelationContext); err != nil {
		return fmt.Errorf("telemetry context: %w", err)
	}
	r.Labels, r.MetricName = SanitizeLabels(r.Labels), SanitizeText(r.MetricName)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if _, ok := fr.sealed[r.CorrelationID]; ok {
		return fmt.Errorf("%w: %s", ErrRecorderSealed, r.CorrelationID)
	}
	fr.touchCorrelationLocked(r.CorrelationID)
	fr.telemetry[r.CorrelationID] = append(fr.telemetry[r.CorrelationID], r)
	return nil
}

// RecordExecution records an entry to Channel 2 (Execution).
func (fr *FlightRecorder) RecordExecution(r ExecutionRecord) error {
	if err := validateContext(r.CorrelationContext); err != nil {
		return fmt.Errorf("execution context: %w", err)
	}
	r.Action, r.ResourceRef = SanitizeText(r.Action), SanitizeText(r.ResourceRef)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if _, ok := fr.sealed[r.CorrelationID]; ok {
		return fmt.Errorf("%w: %s", ErrRecorderSealed, r.CorrelationID)
	}
	fr.touchCorrelationLocked(r.CorrelationID)
	fr.execution[r.CorrelationID] = append(fr.execution[r.CorrelationID], r)
	return nil
}

// RecordDecision records an entry to Channel 3 (Decision).
func (fr *FlightRecorder) RecordDecision(r DecisionRecord) error {
	if err := validateContext(r.CorrelationContext); err != nil {
		return fmt.Errorf("decision context: %w", err)
	}
	if err := ValidateSHA256(r.FactsDigest); err != nil {
		return fmt.Errorf("decision facts digest: %w", err)
	}
	r.EvaluatorAppraisal, r.ReasonCode = SanitizeText(r.EvaluatorAppraisal), SanitizeText(r.ReasonCode)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if _, ok := fr.sealed[r.CorrelationID]; ok {
		return fmt.Errorf("%w: %s", ErrRecorderSealed, r.CorrelationID)
	}
	fr.touchCorrelationLocked(r.CorrelationID)
	fr.decision[r.CorrelationID] = append(fr.decision[r.CorrelationID], r)
	return nil
}

// RecordEvidence records an entry to Channel 4 (Evidence).
func (fr *FlightRecorder) RecordEvidence(r EvidenceRecord) error {
	if err := validateContext(r.CorrelationContext); err != nil {
		return fmt.Errorf("evidence context: %w", err)
	}
	if err := ValidateSHA256(r.SubjectDigest); err != nil {
		return fmt.Errorf("evidence subject digest: %w", err)
	}
	if err := ValidateSHA256(r.ProducerKeyDigest); err != nil {
		return fmt.Errorf("evidence producer key digest: %w", err)
	}
	if r.PreviousDigest != "" {
		if err := ValidateSHA256(r.PreviousDigest); err != nil {
			return fmt.Errorf("evidence previous digest: %w", err)
		}
	}
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if _, ok := fr.sealed[r.CorrelationID]; ok {
		return fmt.Errorf("%w: %s", ErrRecorderSealed, r.CorrelationID)
	}
	fr.touchCorrelationLocked(r.CorrelationID)
	fr.evidence[r.CorrelationID] = append(fr.evidence[r.CorrelationID], r)
	return nil
}

func (fr *FlightRecorder) touchCorrelationLocked(cid string) {
	if _, ok := fr.seen[cid]; !ok {
		if len(fr.order) >= fr.capacity {
			evicted := fr.order[0]
			fr.order = fr.order[1:]
			delete(fr.seen, evicted)
			delete(fr.telemetry, evicted)
			delete(fr.execution, evicted)
			delete(fr.decision, evicted)
			delete(fr.evidence, evicted)
			delete(fr.sealed, evicted)
		}
		fr.order = append(fr.order, cid)
		fr.seen[cid] = struct{}{}
	}
}

// VerifyInvariants checks cross-channel vector state invariants for a correlation.
func (fr *FlightRecorder) VerifyInvariants(cid string) (InvariantState, []string, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	if _, ok := fr.seen[cid]; !ok {
		return InvariantStateIndeterminate, nil, fmt.Errorf("%w: %s", ErrCorrelationNotFound, cid)
	}
	return fr.verifyInvariantsLocked(cid)
}

func (fr *FlightRecorder) checkAntiExecute(decs []DecisionRecord, execs []ExecutionRecord) []string {
	var violations []string
	for _, d := range decs {
		if d.Disposition == DispositionDeny || d.Disposition == DispositionQuarantine {
			for _, e := range execs {
				if (e.Timestamp.After(d.Timestamp) || e.Timestamp.Equal(d.Timestamp)) &&
					(e.State == ExecutionStateRunning || e.State == ExecutionStateCompleted) {
					violations = append(violations, fmt.Sprintf("anti_execute: %s reached %s after %s", e.Component, e.State, d.Disposition))
				}
			}
		}
	}
	return violations
}

func (fr *FlightRecorder) checkActiveUntrusted(execs []ExecutionRecord, evs []EvidenceRecord) []string {
	var violations []string
	for _, ev := range evs {
		if ev.State == EvidenceStateRejected || ev.State == EvidenceStateQuarantine {
			for _, e := range execs {
				if e.State == ExecutionStateRunning || e.State == ExecutionStateCompleted {
					violations = append(violations, fmt.Sprintf("active_untrusted: active %s with untrusted evidence %s", e.Component, ev.ReceiptID))
				}
			}
		}
	}
	return violations
}

func (fr *FlightRecorder) checkDecisionPrecedence(decs []DecisionRecord, execs []ExecutionRecord) []string {
	var violations []string
	for _, e := range execs {
		if e.State == ExecutionStateCompleted && len(decs) == 0 {
			violations = append(violations, fmt.Sprintf("decision_missing: %s completed without decision", e.Component))
		}
	}
	return violations
}

func (fr *FlightRecorder) checkEvidenceChaining(evs []EvidenceRecord) []string {
	var violations []string
	for i, ev := range evs {
		if i == 0 && ev.PreviousDigest != "" {
			violations = append(violations, fmt.Sprintf("chain_break: root %s specifies previousDigest", ev.ReceiptID))
		}
		if i > 0 && ev.PreviousDigest != evs[i-1].SubjectDigest {
			violations = append(violations, fmt.Sprintf("chain_break: %s previousDigest mismatch", ev.ReceiptID))
		}
	}
	return violations
}

func (fr *FlightRecorder) verifyInvariantsLocked(cid string) (InvariantState, []string, error) {
	var violations []string
	violations = append(violations, fr.checkAntiExecute(fr.decision[cid], fr.execution[cid])...)
	violations = append(violations, fr.checkActiveUntrusted(fr.execution[cid], fr.evidence[cid])...)
	violations = append(violations, fr.checkDecisionPrecedence(fr.decision[cid], fr.execution[cid])...)
	violations = append(violations, fr.checkEvidenceChaining(fr.evidence[cid])...)
	if len(violations) > 0 {
		return InvariantStateViolated, violations, nil
	}
	return InvariantStateSatisfied, nil, nil
}

// Correlate returns all traces correlated by correlationID and evaluates invariants.
func (fr *FlightRecorder) Correlate(cid string) (*CorrelatedFlightTraces, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	if _, ok := fr.seen[cid]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrCorrelationNotFound, cid)
	}
	invState, violations, _ := fr.verifyInvariantsLocked(cid)
	return &CorrelatedFlightTraces{
		CorrelationID: cid,
		Telemetry:     append([]TelemetryRecord(nil), fr.telemetry[cid]...),
		Execution:     append([]ExecutionRecord(nil), fr.execution[cid]...),
		Decision:      append([]DecisionRecord(nil), fr.decision[cid]...),
		Evidence:      append([]EvidenceRecord(nil), fr.evidence[cid]...),
		State:         invState,
		Violations:    violations,
	}, nil
}

// Seal finalizes the flight record for a correlation, computing a deterministic Merkle root.
func (fr *FlightRecorder) Seal(cid string) (*SealedFlightRecord, error) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if s, ok := fr.sealed[cid]; ok {
		return s, nil
	}
	if _, ok := fr.seen[cid]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrCorrelationNotFound, cid)
	}
	invState, _, _ := fr.verifyInvariantsLocked(cid)
	digest, count, err := fr.computeRootDigestLocked(cid)
	if err != nil {
		return nil, fmt.Errorf("compute root digest: %w", err)
	}
	sealed := &SealedFlightRecord{
		CorrelationID: cid,
		RootDigest:    digest,
		RecordCount:   count,
		State:         invState,
		SealedAt:      time.Now().UTC(),
		ProducerID:    fr.producerID,
	}
	fr.sealed[cid] = sealed
	return sealed, nil
}

func (fr *FlightRecorder) computeRootDigestLocked(cid string) (string, int, error) {
	hasher := sha256.New()
	for _, payload := range []any{fr.telemetry[cid], fr.execution[cid], fr.decision[cid], fr.evidence[cid]} {
		b, err := json.Marshal(payload)
		if err != nil {
			return "", 0, fmt.Errorf("marshal trace: %w", err)
		}
		hasher.Write(b)
	}
	total := len(fr.telemetry[cid]) + len(fr.execution[cid]) + len(fr.decision[cid]) + len(fr.evidence[cid])
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), total, nil
}

// State returns current operational state vector of the FlightRecorder.
func (fr *FlightRecorder) State() RecorderState {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.state
}
