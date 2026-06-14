package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// requiredPredicates maps each artifact kind to its mandatory predicate URNs.
// These are the MUST-carry attestations per AIPACK-SPEC v0.1.1 §6.
// All kinds additionally require PredSLSAProvenance and PredCycloneDXBOM.
var requiredPredicates = map[v1alpha2.ArtifactKind][]string{
	v1alpha2.KindBaseModel: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredAIBOM,
		v1alpha2.PredTrainingResidency,
	},
	v1alpha2.KindLoRA: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredLoRABaseRef,
		v1alpha2.PredLoRAHyperparams,
	},
	v1alpha2.KindFineTune: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredFineTuneHyperparams,
		v1alpha2.PredFineTuneBaseCompat,
	},
	v1alpha2.KindSkill: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredSkillSafetyReview,
		v1alpha2.PredSkillCapabilityDeclaration,
		v1alpha2.PredSkillStaticAnalysis,
	},
	v1alpha2.KindTool: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredToolSchemaValidation,
		v1alpha2.PredToolStaticAnalysis,
	},
	v1alpha2.KindMCPServer: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredMCPSandboxPolicy,
		v1alpha2.PredMCPToolList,
	},
	v1alpha2.KindPromptTemplate: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredPromptContentHash,
		v1alpha2.PredPromptJailbreakResistance,
	},
	v1alpha2.KindGuardrail: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredGuardrailCoverageTest,
		v1alpha2.PredGuardrailFPR,
	},
	v1alpha2.KindRetrievalIndex: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredRetrievalPIIScan,
		v1alpha2.PredRetrievalEmbedRef,
	},
	v1alpha2.KindDataset: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredDatasetProvenance,
		v1alpha2.PredDatasetConsent,
		v1alpha2.PredDatasetLicensing,
	},
	v1alpha2.KindHarness: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredHarnessMethodology,
		v1alpha2.PredHarnessReproducibility,
	},
	v1alpha2.KindEval: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
	},
	v1alpha2.KindWorkflow: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
	},
	v1alpha2.KindPolicyBundle: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
	},
	v1alpha2.KindAgent: {
		v1alpha2.PredSLSAProvenance,
		v1alpha2.PredCycloneDXBOM,
		v1alpha2.PredAgentComposition,
		v1alpha2.PredAgentBehavioralEval,
		v1alpha2.PredAgentPolicyCompliance,
	},
}

// RequiredPredicates returns the MUST-carry attestation predicate URNs for kind.
// The slice is safe to read; callers must not modify it.
func RequiredPredicates(kind v1alpha2.ArtifactKind) []string {
	if preds, ok := requiredPredicates[kind]; ok {
		return preds
	}
	return nil
}

// MissingPredicates computes the set of required predicate URNs absent from present.
// Returns nil when all required predicates are satisfied.
func MissingPredicates(kind v1alpha2.ArtifactKind, present []string) []string {
	required := RequiredPredicates(kind)
	if len(required) == 0 {
		return nil
	}
	presentSet := make(map[string]struct{}, len(present))
	for _, p := range present {
		presentSet[p] = struct{}{}
	}
	var missing []string
	for _, r := range required {
		if _, ok := presentSet[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
}

// ValidatePredicates returns AIPACK-ATTEST-001 if any required predicate is absent.
func ValidatePredicates(kind v1alpha2.ArtifactKind, present []string) error {
	missing := MissingPredicates(kind, present)
	if len(missing) == 0 {
		return nil
	}
	return newErr(ErrMissingPredicate,
		"required attestation predicate(s) absent",
		joinStrings(missing, ", "),
	)
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += sep + s
	}
	return out
}
