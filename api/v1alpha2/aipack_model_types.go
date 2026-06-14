/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// BaseModelSpec describes an A1 BaseModel artifact per AIPACK-SPEC v0.1.1 §3.3.
type BaseModelSpec struct {
	// Architecture is the model architecture identifier.
	// Examples: "llama3", "mistral", "gemma2"
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// ParameterCount is the model parameter count in billions.
	// +optional
	ParameterCount *float64 `json:"parameterCount,omitempty"`

	// ContextLength is the maximum context length in tokens.
	// +optional
	ContextLength *int64 `json:"contextLength,omitempty"`

	// Quantization describes the quantization format, if applicable.
	// Examples: "bf16", "fp8", "int4-awq"
	// +optional
	Quantization string `json:"quantization,omitempty"`

	// License is the model license identifier.
	// +optional
	License string `json:"license,omitempty"`

	// TrainingDataCutoff is the ISO 8601 date of the training data cutoff.
	// +optional
	TrainingDataCutoff string `json:"trainingDataCutoff,omitempty"`

	// SafetyClassification is the safety classification for this model.
	// Values: "general-purpose", "instruction-tuned", "code", "restricted"
	// +optional
	SafetyClassification string `json:"safetyClassification,omitempty"`

	// EmbeddingDimension is the embedding vector dimension for embedding models.
	// Required when this model is used as the embedding backend for a RetrievalIndex.
	// +optional
	EmbeddingDimension *int32 `json:"embeddingDimension,omitempty"`
}

// LoRASpec describes an A2 LoRA adapter artifact per AIPACK-SPEC v0.1.1 §3.3.
type LoRASpec struct {
	// BaseRef is the OCI digest reference to the BaseModel this adapter targets.
	// Required at composition time (AIPACK-COMPAT-001).
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	BaseRef string `json:"baseRef"`

	// Rank is the LoRA rank (r) used during training.
	// +optional
	Rank *int32 `json:"rank,omitempty"`

	// Alpha is the LoRA alpha scaling factor.
	// +optional
	Alpha *float64 `json:"alpha,omitempty"`

	// TargetModules lists the module names the adapter was applied to.
	// Examples: ["q_proj", "v_proj"]
	// +optional
	TargetModules []string `json:"targetModules,omitempty"`

	// TaskType describes the fine-tuning task.
	// Examples: "CAUSAL_LM", "SEQ_2_SEQ_LM", "CLASSIFICATION"
	// +optional
	TaskType string `json:"taskType,omitempty"`

	// License is the adapter license identifier.
	// +optional
	License string `json:"license,omitempty"`
}

// FineTuneSpec describes an A3 FineTune artifact per AIPACK-SPEC v0.1.1 §3.3.
type FineTuneSpec struct {
	// BaseRef is the OCI digest reference to the BaseModel that was fine-tuned.
	// Required (AIPACK-COMPAT-001).
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	BaseRef string `json:"baseRef"`

	// Technique describes the fine-tuning technique used.
	// Examples: "sft", "dpo", "rlhf", "qlora"
	// +optional
	Technique string `json:"technique,omitempty"`

	// TrainingDataRef is the OCI digest reference to the Dataset artifact used.
	// +optional
	TrainingDataRef string `json:"trainingDataRef,omitempty"`

	// EpochCount is the number of training epochs.
	// +optional
	EpochCount *int32 `json:"epochCount,omitempty"`

	// SafetyRetained declares whether safety alignment was preserved.
	// Must be backed by attestation urn:finetune:safety-retention:v1.
	// +optional
	SafetyRetained *bool `json:"safetyRetained,omitempty"`

	// License is the fine-tuned model license identifier.
	// +optional
	License string `json:"license,omitempty"`

	// ContextLength is the maximum context length if it differs from the base model.
	// +optional
	ContextLength *int64 `json:"contextLength,omitempty"`
}
