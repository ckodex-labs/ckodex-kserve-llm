/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package aipack — operational gateway validators.
// These functions accept v1alpha2 public types and bridge to internal operational logic.
// Concrete implementations are in the corresponding operational files (lineage.go, etc.).
package aipack

import (
	"time"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ValidateLineageEnvelope validates a lineage envelope from the v1alpha2 API layer.
// Returns ErrLineageEnvelopeMissing when envelope is nil per §11.
func ValidateLineageEnvelope(env *v1alpha2.AIPackLineageEnvelope) error {
	if env == nil {
		return newErr(ErrLineageEnvelopeMissing, "lineage envelope is required (AIPACK-LIN-001)", "")
	}
	// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §11 — validate source ref + hash
	return nil
}

// ValidateBlastRadius returns ErrBlastRadiusExceeded when actual exceeds the declared max.
// Per AIPACK-SPEC v0.1.1 §12.
func ValidateBlastRadius(actual, max int) error {
	if actual > max {
		return newErr(ErrBlastRadiusExceeded,
			"blast radius exceeds declared maximum (AIPACK-BLAST-001)",
			"",
		)
	}
	return nil
}

// CheckRVBandBlock returns ErrRVRedBandBlocked when the band is RED and no derogation
// attestation is present, per AIPACK-SPEC v0.1.1 §13.4.
func CheckRVBandBlock(band v1alpha2.RVBand, hasDerogation bool) error {
	if band == v1alpha2.RVBandRed && !hasDerogation {
		return newErr(ErrRVRedBandBlocked,
			"RED risk band blocks composition without signed profile-derogation attestation (AIPACK-RV-002)",
			"",
		)
	}
	return nil
}

// ValidateDeprecationState validates the deprecation state of an artifact per §16.
// Returns ErrDeprecationBlocked for Deprecated phase artifacts.
// Returns ErrSunsetExpired for EndOfLife artifacts whose sunset date has passed.
// Nil notice = active artifact, always passes.
func ValidateDeprecationState(notice *v1alpha2.AIPackDeprecationNotice) error {
	if notice == nil {
		return nil
	}
	switch notice.Phase {
	case v1alpha2.DeprecationPhaseDeprecated:
		return newErr(ErrDeprecationBlocked,
			"artifact is in Deprecated phase and cannot be composed (AIPACK-DEP-001)",
			string(notice.Phase),
		)
	case v1alpha2.DeprecationPhaseEndOfLife:
		if notice.SunsetDate != "" {
			t, err := time.Parse("2006-01-02", notice.SunsetDate)
			if err == nil && time.Now().After(t) {
				return newErr(ErrSunsetExpired,
					"artifact sunset date has passed (AIPACK-DEP-002)",
					notice.SunsetDate,
				)
			}
		}
		// EndOfLife without a parsed past date — still block unless derogation present.
		if notice.DerogationRef == "" {
			return newErr(ErrDeprecationBlocked,
				"artifact is in EndOfLife phase without sunset derogation (AIPACK-DEP-001)",
				string(notice.Phase),
			)
		}
	}
	return nil
}

// ValidateAirGapBundle validates a v1alpha2 AIPackAirGapBundle per §17.
// Returns ErrAirGapTrustRootMissing when TrustRootRef is absent.
// Returns ErrAirGapTSAMissing when TSACertRef is absent.
func ValidateAirGapBundle(bundle *v1alpha2.AIPackAirGapBundle) error {
	if bundle == nil {
		return newErr(ErrAirGapBundleExpired, "air-gap bundle is nil (AIPACK-AIRGAP-001)", "")
	}
	if bundle.TrustRootRef == "" {
		return newErr(ErrAirGapTrustRootMissing,
			"air-gap bundle requires trust root OCI ref (AIPACK-AIRGAP-002)",
			"",
		)
	}
	if bundle.TSACertRef == "" {
		return newErr(ErrAirGapTSAMissing,
			"air-gap bundle requires offline TSA certificate ref (AIPACK-AIRGAP-003)",
			"",
		)
	}
	// TODO(ckodex): implement per AIPACK-SPEC v0.1.1 §17 — verify ValidUntil window
	return nil
}

// ValidateCompositionPattern returns ErrManifoldDistanceExceeded when the pattern name
// is not in the 7 canonical archetypes per §18.
func ValidateCompositionPattern(name string) error {
	known := map[string]bool{
		string(PatternBaselineAgent):   true,
		string(PatternRAGAgent):        true,
		string(PatternToolAgent):       true,
		string(PatternGuardedAgent):    true,
		string(PatternWorkflowAgent):   true,
		string(PatternComplianceAgent): true,
		string(PatternFullStackAgent):  true,
		// Common aliases
		"rag-retriever": true,
	}
	if !known[name] {
		return newErr(ErrManifoldDistanceExceeded,
			"composition pattern not in canonical archetype set (AIPACK-PATTERN-002)",
			name,
		)
	}
	return nil
}

// EvaluatePolicyBundle evaluates an AIPackPolicySpec against a given artifact kind.
// Returns ErrProfileFamilyDenied or ErrProfilePredicateDenied on violations.
// Uses the ForbiddenArtifactTypes + AllowedArtifactTypes (deny-all sentinel) evaluation
// per §19.3 steps 3-4.
func EvaluatePolicyBundle(policy *v1alpha2.AIPackPolicySpec, kind v1alpha2.ArtifactKind) error {
	if policy == nil {
		return nil
	}
	// Step 3: forbidden kinds
	for _, f := range policy.ForbiddenArtifactTypes {
		if f == kind {
			return newErr(ErrProfileFamilyDenied,
				"artifact kind is forbidden by policy (AIPACK-PROFILE-001)",
				string(kind),
			)
		}
	}
	// Step 4: allowed kinds allowlist + deny-all sentinel
	// Note: AllowedArtifactTypes == nil means field absent (allow-all).
	// AllowedArtifactTypes set to an empty non-nil slice = deny-all sentinel (§19.2).
	if policy.AllowedArtifactTypes != nil {
		allowed := false
		for _, a := range policy.AllowedArtifactTypes {
			if a == kind {
				allowed = true
				break
			}
		}
		if !allowed {
			return newErr(ErrProfileFamilyDenied,
				"artifact kind not in policy allowedArtifactTypes (AIPACK-PROFILE-001)",
				string(kind),
			)
		}
	}
	// TODO(ckodex): implement full §19.3 — families, requiredPredicates, maxRiskBand
	return nil
}

// ValidateQuarantineTrigger returns ErrQuarantineTriggerFired when the trigger has fired
// or ErrQuarantineEscalationFail when escalation failed per §21.
func ValidateQuarantineTrigger(trigger *v1alpha2.AIPackQuarantineTrigger) error {
	if trigger == nil {
		return nil
	}
	if trigger.EscalationFail {
		return newErr(ErrQuarantineEscalationFail,
			"quarantine escalation protocol failed (AIPACK-TRIGGER-002)",
			trigger.Reason,
		)
	}
	if trigger.Fired {
		return newErr(ErrQuarantineTriggerFired,
			"quarantine trigger has fired (AIPACK-TRIGGER-001)",
			trigger.Reason,
		)
	}
	return nil
}

// ValidateVADClass returns ErrVADClassUnknown when the given class string is not one
// of the 6 canonical VAD classes per §22.
func ValidateVADClass(class string) error {
	known := map[string]bool{
		string(VADClassPromptInjection):     true,
		string(VADClassJailbreak):           true,
		string(VADClassDataPoisoning):       true,
		string(VADClassModelInversion):      true,
		string(VADClassMembershipInference): true,
		string(VADClassAdversarialInput):    true,
	}
	if !known[class] {
		return newErr(ErrVADClassUnknown,
			"unknown VAD class (AIPACK-VAD-002)",
			class,
		)
	}
	return nil
}

// ValidateOutlierSignal returns ErrOutlierUnacknowledged when an outlier signal is
// present but not acknowledged per §14.
// A nil signal is not an error (no outlier detected).
func ValidateOutlierSignal(signal *v1alpha2.AIPackOutlierSignal) error {
	if signal == nil {
		return nil
	}
	if !signal.Acknowledged {
		return newErr(ErrOutlierUnacknowledged,
			"outlier signal must be acknowledged before promotion (AIPACK-OUTLIER-001)",
			signal.Category,
		)
	}
	return nil
}
