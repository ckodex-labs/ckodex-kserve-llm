package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// Predefined policy profile URNs from AIPACK-SPEC v0.1.1 §19.4.
const (
	ProfileFedRAMPModerateAI = "urn:aipack:profile:fedramp-moderate-ai:v1"
	ProfileCMMCL3Defense     = "urn:aipack:profile:cmmc-l3-defense:v1"
	ProfileEUAIActHighRisk   = "urn:aipack:profile:eu-ai-act-high-risk:v1"
	ProfileHIPAAClinical     = "urn:aipack:profile:hipaa-clinical:v1"
)

// EvaluationResult holds a policy evaluation outcome.
type EvaluationResult struct {
	Allowed bool
	// DenyCode is the error code when Allowed is false.
	DenyCode ErrorCode
	// Reason is the human-readable denial reason.
	Reason string
}

// EvaluatePolicy evaluates a PolicyBundleSpec against an AIPack artifact per §19.3.
// Evaluation order (normative):
//  1. forbiddenFamilies   → DENY (AIPACK-PROFILE-001)
//  2. allowedFamilies     → DENY if non-empty and family not listed (AIPACK-PROFILE-001)
//  3. forbiddenArtifactTypes → DENY (AIPACK-PROFILE-001)
//  4. allowedArtifactTypes → DENY if non-empty and kind not listed (AIPACK-PROFILE-001)
//     empty array [] = deny-all sentinel (absent field = allow-all per §19.2 invariant)
//  5. requiredPredicates  → DENY if any absent (AIPACK-PROFILE-002)
//  6. maxRiskBand         → DENY if artifact band exceeds limit (AIPACK-RV-002)
func EvaluatePolicy(
	policy *v1alpha2.PolicyBundleSpec,
	kind v1alpha2.ArtifactKind,
	family v1alpha2.ArtifactFamily,
	presentPredicates []string,
	riskBand v1alpha2.RVBand,
) EvaluationResult {
	if policy == nil {
		return EvaluationResult{Allowed: true}
	}
	if denial := evaluateFamilyPolicy(policy, family); denial != nil {
		return *denial
	}
	if denial := evaluateArtifactKindPolicy(policy, kind); denial != nil {
		return *denial
	}
	if denial := evaluatePredicatePolicy(policy, presentPredicates); denial != nil {
		return *denial
	}
	if denial := evaluateRiskPolicy(policy, riskBand); denial != nil {
		return *denial
	}
	return EvaluationResult{Allowed: true}
}

func evaluateFamilyPolicy(policy *v1alpha2.PolicyBundleSpec, family v1alpha2.ArtifactFamily) *EvaluationResult {
	for _, ff := range policy.ForbiddenFamilies {
		if ff == family {
			return &EvaluationResult{
				Allowed:  false,
				DenyCode: ErrProfileFamilyDenied,
				Reason:   "family " + string(family) + " is forbidden by policy",
			}
		}
	}
	if len(policy.AllowedFamilies) > 0 && !familyInList(family, policy.AllowedFamilies) {
		return &EvaluationResult{
			Allowed:  false,
			DenyCode: ErrProfileFamilyDenied,
			Reason:   "family " + string(family) + " is not in policy allowedFamilies",
		}
	}
	return nil
}

func evaluateArtifactKindPolicy(policy *v1alpha2.PolicyBundleSpec, kind v1alpha2.ArtifactKind) *EvaluationResult {
	for _, ft := range policy.ForbiddenArtifactTypes {
		if ft == kind {
			return &EvaluationResult{
				Allowed:  false,
				DenyCode: ErrProfileFamilyDenied,
				Reason:   "kind " + string(kind) + " is forbidden by policy",
			}
		}
	}
	if policy.AllowedArtifactTypes != nil && !kindInList(kind, policy.AllowedArtifactTypes) {
		return &EvaluationResult{
			Allowed:  false,
			DenyCode: ErrProfileFamilyDenied,
			Reason:   "kind " + string(kind) + " is not in policy allowedArtifactTypes",
		}
	}
	return nil
}

func evaluatePredicatePolicy(policy *v1alpha2.PolicyBundleSpec, presentPredicates []string) *EvaluationResult {
	for _, req := range policy.RequiredPredicates {
		if !stringInList(req, presentPredicates) {
			return &EvaluationResult{
				Allowed:  false,
				DenyCode: ErrProfilePredicateDenied,
				Reason:   "required predicate absent: " + req,
			}
		}
	}
	return nil
}

func evaluateRiskPolicy(policy *v1alpha2.PolicyBundleSpec, riskBand v1alpha2.RVBand) *EvaluationResult {
	if policy.MaxRiskBand != "" && (!validRVBand(riskBand) || !validRVBand(policy.MaxRiskBand) || riskBandExceeds(riskBand, policy.MaxRiskBand)) {
		return &EvaluationResult{
			Allowed:  false,
			DenyCode: ErrRVRedBandBlocked,
			Reason:   "artifact risk band " + string(riskBand) + " exceeds policy maxRiskBand " + string(policy.MaxRiskBand),
		}
	}
	return nil
}

func familyInList(f v1alpha2.ArtifactFamily, list []v1alpha2.ArtifactFamily) bool {
	for _, l := range list {
		if l == f {
			return true
		}
	}
	return false
}

func kindInList(k v1alpha2.ArtifactKind, list []v1alpha2.ArtifactKind) bool {
	for _, l := range list {
		if l == k {
			return true
		}
	}
	return false
}

func stringInList(s string, list []string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}
	return false
}

// rvBandOrder defines the ordering for risk band comparison (lower index = lower risk).
var rvBandOrder = map[v1alpha2.RVBand]int{
	v1alpha2.RVBandGreen:  0,
	v1alpha2.RVBandYellow: 1,
	v1alpha2.RVBandOrange: 2,
	v1alpha2.RVBandRed:    3,
}

func riskBandExceeds(actual, limit v1alpha2.RVBand) bool {
	ao, actualOK := rvBandOrder[actual]
	lo, limitOK := rvBandOrder[limit]
	if !actualOK || !limitOK {
		return true
	}
	return ao > lo
}

func validRVBand(band v1alpha2.RVBand) bool {
	_, ok := rvBandOrder[band]
	return ok
}
