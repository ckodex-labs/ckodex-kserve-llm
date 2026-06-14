/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// HarnessSpec describes an A11 Harness artifact per AIPACK-SPEC v0.1.1 §3.3.
// A Harness is a reusable evaluation suite (benchmarks, test sets, scoring rubrics).
type HarnessSpec struct {
	// EvalType declares the type of evaluation this harness performs.
	// Examples: "accuracy", "safety", "red-team", "alignment", "robustness"
	// +optional
	EvalType string `json:"evalType,omitempty"`

	// TaskCount is the number of evaluation tasks in the harness.
	// +optional
	TaskCount *int32 `json:"taskCount,omitempty"`

	// Reproducible declares whether the harness produces reproducible scores.
	// Backed by attestation urn:eval-suite:reproducibility:v1.
	// +optional
	Reproducible *bool `json:"reproducible,omitempty"`

	// ReferenceOutputsRef is the OCI digest reference to a Dataset artifact
	// containing ground-truth / reference outputs.
	// Backed by attestation urn:eval-suite:reference-outputs:v1.
	// +optional
	ReferenceOutputsRef string `json:"referenceOutputsRef,omitempty"`

	// ScoringMethod declares the scoring methodology.
	// Examples: "exact-match", "llm-judge", "human-eval", "rouge", "bleu"
	// +optional
	ScoringMethod string `json:"scoringMethod,omitempty"`

	// MethodologyRef is a URI pointing to the methodology document.
	// Backed by attestation urn:eval-suite:methodology:v1.
	// +optional
	MethodologyRef string `json:"methodologyRef,omitempty"`
}

// EvalSpec describes an A12 Eval artifact per AIPACK-SPEC v0.1.1 §3.3.
// An Eval is a specific evaluation run result (scores, traces, leaderboard entry).
type EvalSpec struct {
	// HarnessRef is the OCI digest reference to the Harness artifact used.
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	HarnessRef string `json:"harnessRef"`

	// SubjectRef is the OCI digest reference to the artifact being evaluated.
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	SubjectRef string `json:"subjectRef"`

	// PrimaryScore is the primary aggregate score from the evaluation.
	// +optional
	PrimaryScore *float64 `json:"primaryScore,omitempty"`

	// ScoreUnit is the unit of the primary score.
	// Examples: "accuracy", "pass@k", "mmlu-%", "rouge-l"
	// +optional
	ScoreUnit string `json:"scoreUnit,omitempty"`

	// TaskScores maps per-task names to their individual scores.
	// +optional
	TaskScores map[string]float64 `json:"taskScores,omitempty"`

	// RunEnvironment describes the hardware/software environment of the eval run.
	// Examples: "H100-80GB-SXM / CUDA 12.4 / vLLM 0.23.0"
	// +optional
	RunEnvironment string `json:"runEnvironment,omitempty"`

	// RunDate is the ISO 8601 date of the evaluation run.
	// +optional
	RunDate string `json:"runDate,omitempty"`

	// Reproducible declares whether this eval run is reproducible.
	// +optional
	Reproducible *bool `json:"reproducible,omitempty"`
}
