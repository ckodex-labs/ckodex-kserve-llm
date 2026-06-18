/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"fmt"
	"strings"
)

// OIS v0.1 Constants and URN Strategy.

const (
	OISURNPrefix = "urn:ois:"
	OISAuthority = "ckodex"
)

// OIS Execution Kinds (Section 10)
const (
	ExecKindInference       = "inference"
	ExecKindEmbedding       = "embedding"
	ExecKindRetrieval       = "retrieval"
	ExecKindRerank          = "rerank"
	ExecKindTool            = "tool"
	ExecKindGuardrail       = "guardrail"
	ExecKindEvaluation      = "evaluation"
	ExecKindPromptRender    = "prompt_render"
	ExecKindContextAssembly = "context_assembly"
	ExecKindPlanning        = "planning"
	ExecKindApproval        = "approval"
	ExecKindMaterialization = "materialization"
	ExecKindChain           = "chain"
	ExecKindAgentStep       = "agent_step"
)

// OIS Reproducibility Classes (Section 28)
const (
	ReproNone          = "none"
	ReproExplanatory   = "explanatory"
	ReproBounded       = "bounded"
	ReproDeterministic = "deterministic"
	ReproAttested      = "attested"
)

// OIS Signal Classes (Section 8.1)
const (
	SignalTrace    = "trace"
	SignalEvent    = "event"
	SignalLog      = "log"
	SignalDecision = "decision"
	SignalReceipt  = "receipt"
	SignalEvidence = "evidence"
	SignalState    = "state"
	SignalMetric   = "metric"
	SignalStream   = "stream"
)

// OIS Profile Types (Section 26)

// ModelAssembly defines the model stack used (Section 16).
type ModelAssembly struct {
	Base         ModelIdentity   `json:"base"`
	Tokenizer    *ModelIdentity  `json:"tokenizer,omitempty"`
	Quantization *QuantProfile   `json:"quantization,omitempty"`
	Adapters     []ModelIdentity `json:"adapters,omitempty"`
}

type ModelIdentity struct {
	ID        string `json:"id"`
	URN       string `json:"urn,omitempty"`
	Version   string `json:"version,omitempty"`
	Publisher string `json:"publisher,omitempty"`
}

type QuantProfile struct {
	ID     string `json:"id"`
	Method string `json:"method,omitempty"`
	Bits   int    `json:"bits,omitempty"`
}

// ContentMessage defines a single OIS message (Section 17).
type ContentMessage struct {
	Role     string        `json:"role"`
	Parts    []ContentPart `json:"parts"`
	ToolID   string        `json:"tool_id,omitempty"`
	ToolCall *ToolCall     `json:"tool_call,omitempty"`
}

type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"` // Base64 for binary
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"` // JSON string
}

// PerformanceMetrics defines OIS timing fields (Section 18).
type PerformanceMetrics struct {
	LatencyMS    int64   `json:"latency.ms,omitempty"`
	QueueMS      int64   `json:"queue.ms,omitempty"`
	FirstTokenMS int64   `json:"first_token.ms,omitempty"`
	PrefillMS    int64   `json:"prefill.ms,omitempty"`
	DecodeMS     int64   `json:"decode.ms,omitempty"`
	TokensPerSec float64 `json:"tokens_per_sec,omitempty"`
}

// OIS Outcome values (Section 20.3)
const (
	OutcomeAllow      = "allow"
	OutcomeDeny       = "deny"
	OutcomeSelect     = "select"
	OutcomeReject     = "reject"
	OutcomeDegrade    = "degrade"
	OutcomeQuarantine = "quarantine"
)

// URN Generates portable OIS identifiers.
func URN(kind, id string) string {
	if strings.HasPrefix(id, OISURNPrefix) {
		return id
	}
	// urn:ois:<kind>:<authority>:<id>
	return fmt.Sprintf("%s%s:%s:%s", OISURNPrefix, kind, OISAuthority, id)
}

// OIS Semantic Attribute Keys for OTEL (Section 9/28)
const (
	// exec.* — execution envelope
	AttrExecID         = "exec.id"
	AttrExecKind       = "exec.kind"
	AttrExecStatus     = "exec.status"
	AttrExecStartTime  = "exec.start_time"
	AttrExecEndTime    = "exec.end_time"
	AttrExecReproClass = "exec.reproducibility_class"
	AttrExecMode       = "exec.mode"
	AttrExecParentID   = "exec.parent_id"
	AttrExecRootID     = "exec.root_id"

	// actor.* — actor identity
	AttrActorType = "actor.type"
	AttrActorID   = "actor.id"
	AttrActorURN  = "actor.urn"
	AttrActorRole = "actor.role"

	// engine.* — runtime environment
	AttrEngineRuntime  = "engine.runtime"
	AttrEngineProvider = "engine.provider"
	AttrEngineURN      = "engine.urn"

	// model.* — model assembly
	AttrModelBaseID      = "model.base.id"
	AttrModelBaseURN     = "model.base.urn"
	AttrModelBaseVersion = "model.base.version"
	AttrModelAdapterID   = "model.adapter.id"

	// cost.* — economic semantics
	AttrCostTokensInput  = "cost.tokens.input"
	AttrCostTokensOutput = "cost.tokens.output"
	AttrCostTokensTotal  = "cost.tokens.total"
	AttrCostUSDTotal     = "cost.usd.total"

	// perf.* — timing and performance
	AttrPerfLatencyMS    = "perf.latency.ms"
	AttrPerfQueueMS      = "perf.queue.ms"
	AttrPerfFirstTokenMS = "perf.first_token.ms"

	// policy.* — evaluations
	AttrPolicyDecision = "policy.decision"
	AttrPolicyBundleID = "policy.bundle.id"

	// privacy.* — sensitivity
	AttrPrivacyRedacted    = "privacy.redacted"
	AttrPrivacyPlaceholder = "__REDACTED__"

	// compat.openinference.* — compatibility mapping
	AttrCompatOISpanKind = "compat.openinference.openinference.span.kind"
)
