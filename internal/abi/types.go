/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package abi

import (
	"errors"
	"time"
)

// Common validation errors.
var (
	ErrInvalidModel       = errors.New("abi: invalid model specification")
	ErrInvalidRuntime     = errors.New("abi: invalid runtime specification")
	ErrInvalidAdapter     = errors.New("abi: invalid adapter specification")
	ErrInvalidTokenizer   = errors.New("abi: invalid tokenizer specification")
	ErrInvalidKVTransfer  = errors.New("abi: invalid kv transfer specification")
	ErrInvalidRouting     = errors.New("abi: invalid routing specification")
	ErrInvalidEvidence    = errors.New("abi: invalid evidence specification")
	ErrInvalidCapability  = errors.New("abi: incomplete or non-total capability matrix")
	ErrInvalidConformance = errors.New("abi: conformance requirement violated")
	ErrInvalidTelemetry   = errors.New("abi: invalid telemetry record")
	ErrInvalidSpec        = errors.New("abi: invalid open inference specification")
)

// EngineKind identifies the supported inference engine family.
type EngineKind string

const (
	EngineKServe     EngineKind = "kserve"
	EngineLLMD       EngineKind = "llm-d"
	EngineVLLM       EngineKind = "vllm"
	EngineTensorFold EngineKind = "tensorfold"
	EngineSGLang     EngineKind = "sglang"
)

// ConformanceTier denotes the engine contract compliance level (CKC-ENG).
type ConformanceTier int

const (
	ConformanceTier0 ConformanceTier = 0 // Experimental / gated
	ConformanceTier1 ConformanceTier = 1 // Served (standard OpenAI + health)
	ConformanceTier2 ConformanceTier = 2 // Routed (Tier 1 + KV EndpointPicker metrics)
	ConformanceTier3 ConformanceTier = 3 // Governed (Tier 2 + KV transfer, LoRA swap, full evidence)
)

// ConformanceStatus is a vector status representing conformance result.
type ConformanceStatus string

const (
	ConformancePass   ConformanceStatus = "pass"
	ConformanceFail   ConformanceStatus = "fail"
	ConformanceExempt ConformanceStatus = "exempt"
)

// CapabilitySupport defines how a backend provides a runtime capability.
type CapabilitySupport string

const (
	CapabilitySupported   CapabilitySupport = "supported"
	CapabilityUnsupported CapabilitySupport = "unsupported"
	CapabilityEmulated    CapabilitySupport = "emulated"
)

// Vector State Enums (Vector state, not booleans).

// LifecycleState defines the operational phase of a workload or resource.
type LifecycleState string

const (
	LifecycleUnspecified  LifecycleState = "unspecified"
	LifecycleInitializing LifecycleState = "initializing"
	LifecycleReady        LifecycleState = "ready"
	LifecycleActive       LifecycleState = "active"
	LifecycleDegraded     LifecycleState = "degraded"
	LifecycleTerminating  LifecycleState = "terminating"
	LifecycleTerminated   LifecycleState = "terminated"
	LifecycleFault        LifecycleState = "fault"
)

// SyncState defines synchronization alignment with declared state.
type SyncState string

const (
	SyncUnknown      SyncState = "unknown"
	SyncSynchronized SyncState = "synchronized"
	SyncReconciling  SyncState = "reconciling"
	SyncDrifted      SyncState = "drifted"
	SyncStale        SyncState = "stale"
)

// HealthState captures the health evaluation vector.
type HealthState string

const (
	HealthUnknown     HealthState = "unknown"
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthUnhealthy   HealthState = "unhealthy"
	HealthQuarantined HealthState = "quarantined"
)

// AttestationState captures cryptographic provenance and attestation status.
type AttestationState string

const (
	AttestationUnspecified AttestationState = "unspecified"
	AttestationAsserted    AttestationState = "asserted"
	AttestationVerified    AttestationState = "verified"
	AttestationFailed      AttestationState = "failed"
	AttestationRevoked     AttestationState = "revoked"
)

// StateVector encapsulates multi-dimensional state (Vector state, not booleans).
type StateVector struct {
	Lifecycle   LifecycleState   `json:"lifecycle"`
	Sync        SyncState        `json:"sync"`
	Health      HealthState      `json:"health"`
	Attestation AttestationState `json:"attestation"`
	ObservedAt  time.Time        `json:"observed_at"`
}

// ModelDescriptor declares immutable metadata and architecture constraints of a model.
type ModelDescriptor struct {
	ModelID         string `json:"model_id"`
	Family          string `json:"family"`
	Architecture    string `json:"architecture"`
	ContextLength   int64  `json:"context_length"`
	ParametersCount int64  `json:"parameters_count"`
	Precision       string `json:"precision"`
	Quantization    string `json:"quantization,omitempty"`
	ContentDigest   string `json:"content_digest"`
}

// ModelSpec declares declared model intent and retrieval coordinates.
type ModelSpec struct {
	Descriptor ModelDescriptor   `json:"descriptor"`
	SourceURI  string            `json:"source_uri"`
	Revision   string            `json:"revision"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// AcceleratorType defines hardware accelerator classes.
type AcceleratorType string

const (
	AcceleratorCUDA   AcceleratorType = "cuda"
	AcceleratorROCm   AcceleratorType = "rocm"
	AcceleratorTPU    AcceleratorType = "tpu"
	AcceleratorNeuron AcceleratorType = "neuron"
	AcceleratorCPU    AcceleratorType = "cpu"
)

// ParallelismSpec defines distributed execution topology.
type ParallelismSpec struct {
	TensorParallel   int32 `json:"tensor_parallel"`
	PipelineParallel int32 `json:"pipeline_parallel"`
	DataParallel     int32 `json:"data_parallel"`
	ExpertParallel   int32 `json:"expert_parallel"`
}

// RuntimeDescriptor describes a concrete engine runtime implementation.
type RuntimeDescriptor struct {
	Engine          EngineKind      `json:"engine"`
	Version         string          `json:"version"`
	ImageDigest     string          `json:"image_digest"`
	ConformanceTier ConformanceTier `json:"conformance_tier"`
}

// RuntimeSpec configures compute allocation and runtime constraints.
type RuntimeSpec struct {
	Descriptor       RuntimeDescriptor `json:"descriptor"`
	Accelerator      AcceleratorType   `json:"accelerator"`
	AcceleratorCount int32             `json:"accelerator_count"`
	MemoryBytes      int64             `json:"memory_bytes"`
	Parallelism      ParallelismSpec   `json:"parallelism"`
	ConcurrencyLimit int32             `json:"concurrency_limit"`
}

// AdapterType classifies the adapter technique.
type AdapterType string

const (
	AdapterLoRA         AdapterType = "lora"
	AdapterQLoRA        AdapterType = "qlora"
	AdapterPromptTuning AdapterType = "prompt_tuning"
	AdapterPrefixTuning AdapterType = "prefix_tuning"
)

// AdapterHotSwapPolicy specifies runtime swap semantics.
type AdapterHotSwapPolicy string

const (
	HotSwapImmediate AdapterHotSwapPolicy = "immediate"
	HotSwapDrained   AdapterHotSwapPolicy = "drained"
	HotSwapPreloaded AdapterHotSwapPolicy = "preloaded"
)

// AdapterDescriptor defines immutable adapter specifications.
type AdapterDescriptor struct {
	AdapterID     string      `json:"adapter_id"`
	Type          AdapterType `json:"type"`
	BaseDigest    string      `json:"base_digest"`
	Rank          int32       `json:"rank"`
	Alpha         float64     `json:"alpha"`
	TargetModules []string    `json:"target_modules"`
	Digest        string      `json:"digest"`
}

// AdapterSpec specifies adapter configuration and storage location.
type AdapterSpec struct {
	Descriptor    AdapterDescriptor    `json:"descriptor"`
	HotSwapPolicy AdapterHotSwapPolicy `json:"hot_swap_policy"`
	WeightsURI    string               `json:"weights_uri"`
}

// TokenizerKind specifies tokenizer algorithms.
type TokenizerKind string

const (
	TokenizerBPE           TokenizerKind = "bpe"
	TokenizerWordPiece     TokenizerKind = "wordpiece"
	TokenizerSentencePiece TokenizerKind = "sentencepiece"
	TokenizerUnigram       TokenizerKind = "unigram"
)

// SpecialTokens defines delimiters and template directives.
type SpecialTokens struct {
	BOS          string `json:"bos,omitempty"`
	EOS          string `json:"eos,omitempty"`
	PAD          string `json:"pad,omitempty"`
	UNK          string `json:"unk,omitempty"`
	ChatTemplate string `json:"chat_template,omitempty"`
}

// TokenizerDescriptor provides static tokenizer characteristics.
type TokenizerDescriptor struct {
	Kind             TokenizerKind `json:"kind"`
	VocabularySize   int32         `json:"vocabulary_size"`
	TruncationLength int64         `json:"truncation_length"`
	SpecialTokens    SpecialTokens `json:"special_tokens"`
}

// TokenizerSpec defines tokenizer deployment coordinates.
type TokenizerSpec struct {
	Descriptor TokenizerDescriptor `json:"descriptor"`
	ConfigURI  string              `json:"config_uri"`
}

// KVTransferProtocol denotes the wire transport used for KV cache mobility.
type KVTransferProtocol string

const (
	KVProtocolNixl         KVTransferProtocol = "nixl"
	KVProtocolRDMA         KVTransferProtocol = "rdma"
	KVProtocolGRPC         KVTransferProtocol = "grpc"
	KVProtocolSharedMemory KVTransferProtocol = "shm"
)

// KVTransferState represents the current state of a KV transfer stream.
type KVTransferState string

const (
	KVTransferIdle        KVTransferState = "idle"
	KVTransferHandshaking KVTransferState = "handshaking"
	KVTransferStreaming   KVTransferState = "streaming"
	KVTransferCompleted   KVTransferState = "completed"
	KVTransferFailed      KVTransferState = "failed"
	KVTransferAborted     KVTransferState = "aborted"
)

// KVCacheDtype denotes precision for KV cache tensors.
type KVCacheDtype string

const (
	KVDtypeFP16 KVCacheDtype = "fp16"
	KVDtypeBF16 KVCacheDtype = "bf16"
	KVDtypeFP8  KVCacheDtype = "fp8"
	KVDtypeINT8 KVCacheDtype = "int8"
	KVDtypeINT4 KVCacheDtype = "int4"
)

// KVTransferDescriptor declares KV transfer capabilities and chunk parameters.
type KVTransferDescriptor struct {
	Protocol            KVTransferProtocol `json:"protocol"`
	CacheDtype          KVCacheDtype       `json:"cache_dtype"`
	BlockSize           int32              `json:"block_size"`
	PageSize            int32              `json:"page_size"`
	ChannelCapacityByte int64              `json:"channel_capacity_byte"`
}

// KVTransferSpec defines active KV transfer peering.
type KVTransferSpec struct {
	Descriptor     KVTransferDescriptor `json:"descriptor"`
	RemoteEndpoint string               `json:"remote_endpoint"`
	SessionID      string               `json:"session_id"`
}

// RoutingPolicy defines traffic distribution strategies.
type RoutingPolicy string

const (
	RoutingPolicyRoundRobin       RoutingPolicy = "round_robin"
	RoutingPolicyLeastConnections RoutingPolicy = "least_connections"
	RoutingPolicyPrefixCacheAware RoutingPolicy = "prefix_cache_aware"
	RoutingPolicyKVAware          RoutingPolicy = "kv_aware"
	RoutingPolicyLatencyBiased    RoutingPolicy = "latency_biased"
)

// RoutingOutcome specifies policy arbitration outcomes.
type RoutingOutcome string

const (
	OutcomeAllow      RoutingOutcome = "allow"
	OutcomeDeny       RoutingOutcome = "deny"
	OutcomeDegrade    RoutingOutcome = "degrade"
	OutcomeQuarantine RoutingOutcome = "quarantine"
	OutcomeRedirect   RoutingOutcome = "redirect"
)

// EndpointPickerMetrics provides standard metrics for KV-aware scheduling.
type EndpointPickerMetrics struct {
	Endpoint                    string `json:"endpoint"`
	QueueDepth                  int32  `json:"queue_depth"`
	RunningRequests             int32  `json:"running_requests"`
	KVCacheUtilizationBasePoint int32  `json:"kv_cache_utilization_bp"` // 0..10000 basis points
	ActiveAdaptersCount         int32  `json:"active_adapters_count"`
}

// RouteTarget declares a selectable downstream destination.
type RouteTarget struct {
	Endpoint string      `json:"endpoint"`
	Weight   int32       `json:"weight"`
	Priority int32       `json:"priority"`
	State    StateVector `json:"state"`
}

// RoutingSpec governs traffic distribution parameters.
type RoutingSpec struct {
	Policy          RoutingPolicy `json:"policy"`
	Targets         []RouteTarget `json:"targets"`
	FallbackTarget  *RouteTarget  `json:"fallback_target,omitempty"`
	MaxQueueTimeout time.Duration `json:"max_queue_timeout"`
}

// CapabilityMatrix is a total mapping over all governed runtime capabilities.
// Silence is not a declaration: every capability must be explicitly declared.
type CapabilityMatrix struct {
	TensorParallel      CapabilitySupport `json:"tensor_parallel"`
	DataParallel        CapabilitySupport `json:"data_parallel"`
	PipelineParallel    CapabilitySupport `json:"pipeline_parallel"`
	ExpertParallel      CapabilitySupport `json:"expert_parallel"`
	KVCacheDtype        CapabilitySupport `json:"kv_cache_dtype"`
	KVTransfer          CapabilitySupport `json:"kv_transfer"`
	SpeculativeDecoding CapabilitySupport `json:"speculative_decoding"`
	Quantization        CapabilitySupport `json:"quantization"`
	LoRAHotSwap         CapabilitySupport `json:"lora_hot_swap"`
	StructuredOutput    CapabilitySupport `json:"structured_output"`
	ChunkedPrefill      CapabilitySupport `json:"chunked_prefill"`
}

// EvidenceKind classifies supply-chain and attestation signals.
type EvidenceKind string

const (
	EvidenceDigest       EvidenceKind = "digest"
	EvidenceSignature    EvidenceKind = "signature"
	EvidenceAttestation  EvidenceKind = "attestation"
	EvidenceSBOM         EvidenceKind = "sbom"
	EvidenceAuditReceipt EvidenceKind = "audit_receipt"
)

// EvidenceClaim declares one attested cryptographic claim.
type EvidenceClaim struct {
	ClaimID    string       `json:"claim_id"`
	Kind       EvidenceKind `json:"kind"`
	Issuer     string       `json:"issuer"`
	SubjectURN string       `json:"subject_urn"`
	Algorithm  string       `json:"algorithm"`
	Digest     string       `json:"digest"`
	AssertedAt time.Time    `json:"asserted_at"`
	VerifiedAt *time.Time   `json:"verified_at,omitempty"`
}

// EvidenceRecord aggregates claims for an artifact.
type EvidenceRecord struct {
	Claims []EvidenceClaim  `json:"claims"`
	State  AttestationState `json:"state"`
}

// ConformanceEvaluation encapsulates conformance testing outcomes.
type ConformanceEvaluation struct {
	Tier       ConformanceTier   `json:"tier"`
	Status     ConformanceStatus `json:"status"`
	Violations []string          `json:"violations,omitempty"`
}

// SignalClass classifies observability signal streams.
type SignalClass string

const (
	SignalTrace    SignalClass = "trace"
	SignalEvent    SignalClass = "event"
	SignalLog      SignalClass = "log"
	SignalMetric   SignalClass = "metric"
	SignalReceipt  SignalClass = "receipt"
	SignalEvidence SignalClass = "evidence"
)

// TelemetryMetrics captures latency, throughput, and token counts.
type TelemetryMetrics struct {
	TTFTMicroseconds         int64 `json:"ttft_us"`
	ITLMicroseconds          int64 `json:"itl_us"`
	QueueLatencyMicroseconds int64 `json:"queue_latency_us"`
	PrefillMicroseconds      int64 `json:"prefill_us"`
	DecodeMicroseconds       int64 `json:"decode_us"`
	TotalLatencyMicroseconds int64 `json:"total_latency_us"`
	TokensPerSecondBP        int64 `json:"tokens_per_second_bp"` // basis points (100 = 1 token/sec)
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
}

// TelemetryRecord captures an observable runtime telemetry emission.
type TelemetryRecord struct {
	Signal    SignalClass      `json:"signal"`
	Timestamp time.Time        `json:"timestamp"`
	Metrics   TelemetryMetrics `json:"metrics"`
	TraceID   string           `json:"trace_id,omitempty"`
	SpanID    string           `json:"span_id,omitempty"`
}
