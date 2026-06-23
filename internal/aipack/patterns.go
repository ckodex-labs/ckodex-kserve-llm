package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// CompositionPattern is one of the 7 canonical composition archetypes per AIPACK-SPEC §18.
type CompositionPattern string

const (
	PatternBaselineAgent   CompositionPattern = "baseline-agent"   // P1: model only
	PatternRAGAgent        CompositionPattern = "rag-agent"        // P2: model + retrieval
	PatternToolAgent       CompositionPattern = "tool-agent"       // P3: model + tools
	PatternGuardedAgent    CompositionPattern = "guarded-agent"    // P4: model + guardrails
	PatternWorkflowAgent   CompositionPattern = "workflow-agent"   // P5: model + workflow
	PatternComplianceAgent CompositionPattern = "compliance-agent" // P6: model + policy bundle
	PatternFullStackAgent  CompositionPattern = "full-stack-agent" // P7: all slots
)

// InferPattern returns the closest matching canonical composition pattern for the given spec.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §18 — slot presence → pattern classification
func InferPattern(_ *v1alpha2.AIPackComposition) CompositionPattern {
	return PatternBaselineAgent
}

// ManifoldDistance computes the semantic distance between two composition specs.
// Returns a value in [0,1] where 0 = identical and 1 = maximally dissimilar.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §18 — slot-vector Hamming distance
func ManifoldDistance(_, _ *v1alpha2.AIPackComposition) float64 {
	return 0
}

// ValidatePattern returns AIPACK-PATTERN-001 when the composition does not conform to
// the expected canonical pattern.
// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §18 — pattern constraint checking
func ValidatePattern(_ CompositionPattern, _ *v1alpha2.AIPackComposition) error {
	return nil
}
