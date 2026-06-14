package aipack

import (
	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// KindFamilyMap is the normative kind→family mapping per AIPACK-SPEC v0.1.1 §3.5.
// Implementations MUST enforce this mapping when validating a declared Family field.
var KindFamilyMap = map[v1alpha2.ArtifactKind]v1alpha2.ArtifactFamily{
	v1alpha2.KindBaseModel:      v1alpha2.FamilyModel,
	v1alpha2.KindLoRA:           v1alpha2.FamilyModel,
	v1alpha2.KindFineTune:       v1alpha2.FamilyModel,
	v1alpha2.KindSkill:          v1alpha2.FamilyCapability,
	v1alpha2.KindTool:           v1alpha2.FamilyCapability,
	v1alpha2.KindMCPServer:      v1alpha2.FamilyCapability,
	v1alpha2.KindWorkflow:       v1alpha2.FamilyCapability,
	v1alpha2.KindPromptTemplate: v1alpha2.FamilyControl,
	v1alpha2.KindGuardrail:      v1alpha2.FamilyControl,
	v1alpha2.KindPolicyBundle:   v1alpha2.FamilyControl,
	v1alpha2.KindRetrievalIndex: v1alpha2.FamilyKnowledge,
	v1alpha2.KindDataset:        v1alpha2.FamilyKnowledge,
	v1alpha2.KindHarness:        v1alpha2.FamilyAssurance,
	v1alpha2.KindEval:           v1alpha2.FamilyAssurance,
	v1alpha2.KindAgent:          v1alpha2.FamilyComposite,
}

// FamilyForKind returns the canonical family for the given kind, plus a bool indicating
// whether the kind is known (AIPACK-KIND-000).
func FamilyForKind(kind v1alpha2.ArtifactKind) (v1alpha2.ArtifactFamily, bool) {
	f, ok := KindFamilyMap[kind]
	return f, ok
}

// ValidateKind returns an error when kind is not in the 15-kind set (AIPACK-KIND-000).
func ValidateKind(kind v1alpha2.ArtifactKind) error {
	if _, ok := KindFamilyMap[kind]; !ok {
		return newErr(ErrKindUnknown, "unknown artifact kind", string(kind))
	}
	return nil
}

// ValidateFamily returns an error when the declared family does not match the
// canonical §3.5 mapping for the given kind (AIPACK-KIND-001).
func ValidateFamily(kind v1alpha2.ArtifactKind, declared v1alpha2.ArtifactFamily) error {
	canonical, ok := KindFamilyMap[kind]
	if !ok {
		return newErr(ErrKindUnknown, "unknown artifact kind", string(kind))
	}
	if declared != canonical {
		return newErr(ErrFamilyMismatch,
			"declared family does not match §3.5 canonical mapping",
			string(kind)+" → "+string(declared)+" (want "+string(canonical)+")",
		)
	}
	return nil
}

// IsAtomicKind reports whether kind is one of the 14 atomic kinds (A1–A14).
func IsAtomicKind(kind v1alpha2.ArtifactKind) bool {
	return kind != v1alpha2.KindAgent
}

// IsCompositeKind reports whether kind is the Agent composite (C1).
func IsCompositeKind(kind v1alpha2.ArtifactKind) bool {
	return kind == v1alpha2.KindAgent
}
