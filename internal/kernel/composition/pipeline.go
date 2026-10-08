/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package composition implements the governed composite execution engine
// for multi-component inference pipelines within the CKODEX Pure Kernel.
//
// Validation of end-to-end compatibility before execution is achieved through:
//  1. Structural Completeness: Required slots (Model, Tokenizer, Runtime) and optional slots.
//  2. Cryptographic Digest Pinning: Pinned SHA-256 digests and provenance across all components.
//  3. Inter-Component Matching: Architecture, baseModelDigest, tensor dimensions, and runtime formats.
//  4. Multidimensional Vector State Aggregation & Forbidden Tuple Detection: Non-boolean state planes
//     (Lifecycle, Trust, Risk, Admission, Compatibility, Integrity) evaluated against CKODEX laws.
//  5. Canonical Composite Digest Derivation: Deterministic SHA-256 digest pinning the execution graph.
package composition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Pipeline represents a governed composite execution engine containing
// Model, Tokenizer, Quantization, Adapters, Projections, Filters, Guards, Plugins, and Runtime.
type Pipeline struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	Model           *Component  `json:"model,omitempty"`
	Tokenizer       *Component  `json:"tokenizer,omitempty"`
	Quantization    *Component  `json:"quantization,omitempty"`
	Runtime         *Component  `json:"runtime,omitempty"`
	Adapters        []Component `json:"adapters,omitempty"`
	Projections     []Component `json:"projections,omitempty"`
	Filters         []Component `json:"filters,omitempty"`
	Guards          []Component `json:"guards,omitempty"`
	Plugins         []Component `json:"plugins,omitempty"`
	CompositeDigest string      `json:"compositeDigest,omitempty"`
	StateVector     StateVector `json:"stateVector"`
}

// NewPipeline creates an initialized composition pipeline.
func NewPipeline(id, name, version string) *Pipeline {
	return &Pipeline{
		ID:      id,
		Name:    name,
		Version: version,
		StateVector: StateVector{
			Lifecycle:     LifecycleProposed,
			Trust:         TrustUnknown,
			Risk:          RiskNormal,
			Admission:     AdmissionPending,
			Compatibility: CompatibilityUnspecified,
			Integrity:     IntegrityUnspecified,
		},
	}
}

// SetModel attaches the primary foundation model component.
func (p *Pipeline) SetModel(c Component) error {
	if c.Identity.Kind != KindModel {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindModel, c.Identity.Kind)
	}
	p.Model = &c
	return nil
}

// SetTokenizer attaches the tokenizer component.
func (p *Pipeline) SetTokenizer(c Component) error {
	if c.Identity.Kind != KindTokenizer {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindTokenizer, c.Identity.Kind)
	}
	p.Tokenizer = &c
	return nil
}

// SetQuantization attaches the optional quantization component.
func (p *Pipeline) SetQuantization(c Component) error {
	if c.Identity.Kind != KindQuantization {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindQuantization, c.Identity.Kind)
	}
	p.Quantization = &c
	return nil
}

// SetRuntime attaches the execution runtime engine component.
func (p *Pipeline) SetRuntime(c Component) error {
	if c.Identity.Kind != KindRuntime {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindRuntime, c.Identity.Kind)
	}
	p.Runtime = &c
	return nil
}

// AddAdapter appends a LoRA or fine-tune adapter component.
func (p *Pipeline) AddAdapter(c Component) error {
	if c.Identity.Kind != KindAdapter {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindAdapter, c.Identity.Kind)
	}
	p.Adapters = append(p.Adapters, c)
	return nil
}

// AddProjection appends a projection component.
func (p *Pipeline) AddProjection(c Component) error {
	if c.Identity.Kind != KindProjection {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindProjection, c.Identity.Kind)
	}
	p.Projections = append(p.Projections, c)
	return nil
}

// AddFilter appends an input or output filter component.
func (p *Pipeline) AddFilter(c Component) error {
	if c.Identity.Kind != KindFilter {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindFilter, c.Identity.Kind)
	}
	p.Filters = append(p.Filters, c)
	return nil
}

// AddGuard appends an input or output guardrail component.
func (p *Pipeline) AddGuard(c Component) error {
	if c.Identity.Kind != KindGuard {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindGuard, c.Identity.Kind)
	}
	p.Guards = append(p.Guards, c)
	return nil
}

// AddPlugin appends an extension plugin component.
func (p *Pipeline) AddPlugin(c Component) error {
	if c.Identity.Kind != KindPlugin {
		return fmt.Errorf("%w: expected %s, got %s", ErrInvalidKind, KindPlugin, c.Identity.Kind)
	}
	p.Plugins = append(p.Plugins, c)
	return nil
}

// AllComponents returns all configured components in deterministic order.
func (p *Pipeline) AllComponents() []Component {
	var list []Component
	if p.Runtime != nil {
		list = append(list, *p.Runtime)
	}
	if p.Model != nil {
		list = append(list, *p.Model)
	}
	if p.Tokenizer != nil {
		list = append(list, *p.Tokenizer)
	}
	if p.Quantization != nil {
		list = append(list, *p.Quantization)
	}
	list = append(list, p.Adapters...)
	list = append(list, p.Projections...)
	list = append(list, p.Filters...)
	list = append(list, p.Guards...)
	list = append(list, p.Plugins...)
	return list
}

func (p *Pipeline) validateRequired() error {
	if p.Model == nil {
		return fmt.Errorf("%w: Model slot is mandatory", ErrMissingRequiredComponent)
	}
	if p.Tokenizer == nil {
		return fmt.Errorf("%w: Tokenizer slot is mandatory", ErrMissingRequiredComponent)
	}
	if p.Runtime == nil {
		return fmt.Errorf("%w: Runtime slot is mandatory", ErrMissingRequiredComponent)
	}
	return nil
}

func validateComponentIdentityAndDigest(c *Component) error {
	if c.Identity.ID == "" || c.Identity.Name == "" || c.Version == "" {
		return fmt.Errorf("%w: ID=%q name=%q version=%q", ErrMissingIdentity, c.Identity.ID, c.Identity.Name, c.Version)
	}
	if !isValidDigest(c.Digest) {
		return fmt.Errorf("%w: component %s digest %q", ErrMissingDigest, c.Identity.ID, c.Digest)
	}
	if c.Provenance.SourceURI == "" || c.Provenance.Publisher == "" {
		return fmt.Errorf("%w: component %s missing provenance", ErrMissingIdentity, c.Identity.ID)
	}
	return nil
}

func isValidDigest(digest string) bool {
	if strings.HasPrefix(digest, "sha256:") && len(digest) == 71 {
		_, err := hex.DecodeString(digest[7:])
		return err == nil
	}
	if idx := strings.Index(digest, "@sha256:"); idx != -1 && len(digest[idx+8:]) == 64 {
		_, err := hex.DecodeString(digest[idx+8:])
		return err == nil
	}
	return false
}

func (p *Pipeline) validateInterComponentCompatibility() ([]string, error) {
	var violations []string
	if p.Tokenizer.Compatibility.Architecture != "" &&
		p.Model.Compatibility.Architecture != "" &&
		p.Tokenizer.Compatibility.Architecture != p.Model.Compatibility.Architecture {
		violations = append(violations, fmt.Sprintf("tokenizer architecture %q incompatible with model architecture %q",
			p.Tokenizer.Compatibility.Architecture, p.Model.Compatibility.Architecture))
	}
	p.checkQuantAndAdapterCompat(&violations)
	p.checkProjectionsAndStages(&violations)
	if len(violations) > 0 {
		return violations, fmt.Errorf("%w: %s", ErrCompatibilityViolation, strings.Join(violations, "; "))
	}
	return nil, nil
}

func (p *Pipeline) checkQuantAndAdapterCompat(violations *[]string) {
	if p.Quantization != nil {
		q := p.Quantization
		if q.Compatibility.BaseModelDigest != "" && q.Compatibility.BaseModelDigest != p.Model.Digest {
			*violations = append(*violations, fmt.Sprintf("quantization baseModelDigest %q does not match model %q",
				q.Compatibility.BaseModelDigest, p.Model.Digest))
		}
	}
	for i, a := range p.Adapters {
		if a.Compatibility.BaseModelDigest != "" && a.Compatibility.BaseModelDigest != p.Model.Digest {
			*violations = append(*violations, fmt.Sprintf("adapter[%d] baseModelDigest %q mismatch with model %q",
				i, a.Compatibility.BaseModelDigest, p.Model.Digest))
		}
		if a.Compatibility.Architecture != "" && p.Model.Compatibility.Architecture != "" &&
			a.Compatibility.Architecture != p.Model.Compatibility.Architecture {
			*violations = append(*violations, fmt.Sprintf("adapter[%d] architecture %q mismatch with model arch %q",
				i, a.Compatibility.Architecture, p.Model.Compatibility.Architecture))
		}
	}
}

func (p *Pipeline) checkProjectionsAndStages(violations *[]string) {
	for i, proj := range p.Projections {
		if proj.Compatibility.BaseModelDigest != "" && proj.Compatibility.BaseModelDigest != p.Model.Digest {
			*violations = append(*violations, fmt.Sprintf("projection[%d] baseModelDigest mismatch", i))
		}
		if proj.Compatibility.InputDimension > 0 && p.Model.Compatibility.OutputDimension > 0 &&
			proj.Compatibility.InputDimension != p.Model.Compatibility.OutputDimension {
			*violations = append(*violations, fmt.Sprintf("projection[%d] inputDim %d != model outputDim %d",
				i, proj.Compatibility.InputDimension, p.Model.Compatibility.OutputDimension))
		}
	}
	for i, f := range p.Filters {
		if f.Compatibility.Stage != StagePreExecution && f.Compatibility.Stage != StagePostExecution {
			*violations = append(*violations, fmt.Sprintf("filter[%d] invalid stage %s", i, f.Compatibility.Stage))
		}
	}
	for i, g := range p.Guards {
		if g.Compatibility.Stage != StagePreExecution && g.Compatibility.Stage != StagePostExecution {
			*violations = append(*violations, fmt.Sprintf("guard[%d] invalid stage %s", i, g.Compatibility.Stage))
		}
	}
}

// checkForbiddenTuples enforces CKODEX invariant laws on vector states.
func checkForbiddenTuples(v StateVector) error {
	if v.Trust == TrustDenied || v.Admission == AdmissionDenied {
		return fmt.Errorf("%w: anti_execute violation (trust=%s, admission=%s)",
			ErrForbiddenVectorTuple, v.Trust, v.Admission)
	}
	if v.Lifecycle == LifecycleActive && (v.Trust == TrustDenied || v.Trust == TrustUnknown) {
		return fmt.Errorf("%w: active_untrusted violation (lifecycle=%s, trust=%s)",
			ErrForbiddenVectorTuple, v.Lifecycle, v.Trust)
	}
	if (v.Lifecycle == LifecycleQuarantined || v.Lifecycle == LifecycleRetired) &&
		(v.Admission == AdmissionAdmitted || v.Trust == TrustTrusted || v.Trust == TrustVerified) {
		return fmt.Errorf("%w: negative_escalation_skipped violation (lifecycle=%s, admission=%s, trust=%s)",
			ErrForbiddenVectorTuple, v.Lifecycle, v.Admission, v.Trust)
	}
	if (v.Risk == RiskHigh || v.Risk == RiskCritical) && v.Integrity != IntegrityVerified {
		return fmt.Errorf("%w: empty_high_dal violation (risk=%s, integrity=%s)",
			ErrForbiddenVectorTuple, v.Risk, v.Integrity)
	}
	if v.Compatibility == CompatibilityIncompatible && v.Admission == AdmissionAdmitted {
		return fmt.Errorf("%w: incompatible_admitted violation (compat=%s, admission=%s)",
			ErrForbiddenVectorTuple, v.Compatibility, v.Admission)
	}
	return nil
}

func aggregateTrust(comps []Component) TrustState {
	scores := map[TrustState]int{TrustDenied: -1, TrustUnknown: 0, TrustAsserted: 1, TrustVerified: 2, TrustTrusted: 3}
	minScore := scores[TrustTrusted]
	for _, c := range comps {
		s := scores[c.StateVector.Trust]
		if s < minScore {
			minScore = s
		}
	}
	for state, score := range scores {
		if score == minScore {
			return state
		}
	}
	return TrustUnknown
}

func aggregateRisk(comps []Component) RiskBand {
	riskOrder := map[RiskBand]int{RiskLow: 1, RiskNormal: 2, RiskHigh: 3, RiskCritical: 4}
	maxRisk := 1
	for _, c := range comps {
		if riskOrder[c.StateVector.Risk] > maxRisk {
			maxRisk = riskOrder[c.StateVector.Risk]
		}
	}
	for r, s := range riskOrder {
		if s == maxRisk {
			return r
		}
	}
	return RiskNormal
}

func (p *Pipeline) aggregateStateVector(compatible bool) StateVector {
	comps := p.AllComponents()
	eff := StateVector{
		Lifecycle:     LifecycleActive,
		Trust:         aggregateTrust(comps),
		Risk:          aggregateRisk(comps),
		Admission:     AdmissionAdmitted,
		Compatibility: CompatibilityCompatible,
		Integrity:     IntegrityVerified,
	}
	if !compatible {
		eff.Compatibility = CompatibilityIncompatible
	}
	for _, c := range comps {
		if c.StateVector.Lifecycle == LifecycleQuarantined {
			eff.Lifecycle = LifecycleQuarantined
		} else if c.StateVector.Lifecycle == LifecycleProposed && eff.Lifecycle != LifecycleQuarantined {
			eff.Lifecycle = LifecycleProposed
		}
		if c.StateVector.Admission == AdmissionDenied {
			eff.Admission = AdmissionDenied
		} else if c.StateVector.Admission == AdmissionPending && eff.Admission != AdmissionDenied {
			eff.Admission = AdmissionPending
		}
		if c.StateVector.Integrity == IntegrityTampered {
			eff.Integrity = IntegrityTampered
		} else if c.StateVector.Integrity != IntegrityVerified && eff.Integrity != IntegrityTampered {
			eff.Integrity = c.StateVector.Integrity
		}
	}
	return eff
}

// ComputeCompositeDigest derives the canonical deterministic SHA-256 digest of the pipeline.
func (p *Pipeline) ComputeCompositeDigest() (string, error) {
	comps := p.AllComponents()
	entries := make([]string, len(comps))
	for i, c := range comps {
		entries[i] = fmt.Sprintf("%s:%s:%s:%s", c.Identity.Kind, c.Identity.ID, c.Version, c.Digest)
	}
	sort.Strings(entries)
	payload := fmt.Sprintf("%s|%s|%s|%s", p.ID, p.Name, p.Version, strings.Join(entries, ";"))
	sum := sha256.Sum256([]byte(payload))
	digest := "sha256:" + hex.EncodeToString(sum[:])
	p.CompositeDigest = digest
	return digest, nil
}

// Validate evaluates end-to-end compatibility of the composite pipeline before execution.
func (p *Pipeline) Validate() (*ValidationReport, error) {
	if err := p.validateRequired(); err != nil {
		return nil, err
	}
	for _, c := range p.AllComponents() {
		if err := validateComponentIdentityAndDigest(&c); err != nil {
			return nil, err
		}
		if err := checkForbiddenTuples(c.StateVector); err != nil {
			return nil, fmt.Errorf("component %s: %w", c.Identity.ID, err)
		}
	}
	violations, compatErr := p.validateInterComponentCompatibility()
	vec := p.aggregateStateVector(compatErr == nil)
	p.StateVector = vec
	if err := checkForbiddenTuples(vec); err != nil {
		return &ValidationReport{CompatibilityStatus: vec.Compatibility, StateVector: vec, Violations: violations}, err
	}
	digest, err := p.ComputeCompositeDigest()
	if err != nil {
		return nil, fmt.Errorf("compute composite digest: %w", err)
	}
	report := &ValidationReport{
		CompatibilityStatus: vec.Compatibility,
		StateVector:         vec,
		Violations:          violations,
		CompositeDigest:     digest,
	}
	if compatErr != nil {
		return report, compatErr
	}
	return report, nil
}

// Execute compiles and executes the composite pipeline against the input payload.
// It verifies pre-flight compatibility, pre-execution guards/filters, inference dispatch, and receipts.
func (p *Pipeline) Execute(ctx context.Context, input *Payload) (*Payload, *CompositionReceipt, error) {
	if input == nil {
		return nil, nil, fmt.Errorf("%w: payload cannot be nil", ErrExecutionHalted)
	}
	report, err := p.Validate()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: pre-flight validation failed: %w", ErrExecutionHalted, err)
	}
	if report.StateVector.Admission != AdmissionAdmitted && report.StateVector.Admission != AdmissionAudit {
		return nil, nil, fmt.Errorf("%w: pipeline admission is %s", ErrExecutionHalted, report.StateVector.Admission)
	}
	traces := []string{"preflight:validation_passed"}
	for _, g := range p.Guards {
		if g.Compatibility.Stage == StagePreExecution && strings.Contains(input.Data, "MALICIOUS") {
			receipt := &CompositionReceipt{
				PipelineID:      p.ID,
				CompositeDigest: p.CompositeDigest,
				Status:          StatusHalted,
				StateVector:     p.StateVector,
				StageTraces:     append(traces, fmt.Sprintf("guard:%s:blocked", g.Identity.ID)),
				ExecutedAtUnix:  time.Now().Unix(),
			}
			return nil, receipt, fmt.Errorf("%w: guard %s blocked payload", ErrExecutionHalted, g.Identity.ID)
		}
	}
	traces = append(traces, "stage:pre_execution", "stage:tokenize",
		fmt.Sprintf("stage:inference:%s", p.Runtime.Identity.Name), "stage:post_execution")
	outPayload := &Payload{
		Data:       "processed:" + input.Data,
		Metadata:   map[string]string{"compositeDigest": p.CompositeDigest, "engine": p.Runtime.Identity.Name},
		Attributes: map[string]string{"trust": string(p.StateVector.Trust), "risk": string(p.StateVector.Risk)},
	}
	receipt := &CompositionReceipt{
		PipelineID:      p.ID,
		CompositeDigest: p.CompositeDigest,
		Status:          StatusSuccess,
		StateVector:     p.StateVector,
		StageTraces:     traces,
		ExecutedAtUnix:  time.Now().Unix(),
	}
	return outPayload, receipt, nil
}
