package aipack

import (
	"testing"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestEvaluatePolicyEnforcesFamiliesPredicatesAndRisk(t *testing.T) {
	policy := &v1alpha2.PolicyBundleSpec{
		AllowedFamilies:    []v1alpha2.ArtifactFamily{v1alpha2.FamilyModel},
		RequiredPredicates: []string{"urn:test:provenance"},
		MaxRiskBand:        v1alpha2.RVBandYellow,
	}

	tests := []struct {
		name       string
		family     v1alpha2.ArtifactFamily
		predicates []string
		risk       v1alpha2.RVBand
		code       ErrorCode
	}{
		{name: "family", family: v1alpha2.FamilyCapability, predicates: []string{"urn:test:provenance"}, risk: v1alpha2.RVBandGreen, code: ErrProfileFamilyDenied},
		{name: "predicate", family: v1alpha2.FamilyModel, risk: v1alpha2.RVBandGreen, code: ErrProfilePredicateDenied},
		{name: "risk", family: v1alpha2.FamilyModel, predicates: []string{"urn:test:provenance"}, risk: v1alpha2.RVBandOrange, code: ErrRVRedBandBlocked},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := EvaluatePolicy(policy, v1alpha2.KindBaseModel, tc.family, tc.predicates, tc.risk)
			if result.Allowed || result.DenyCode != tc.code {
				t.Fatalf("got allowed=%t code=%q, want denial code %q", result.Allowed, result.DenyCode, tc.code)
			}
		})
	}
}

func TestEvaluatePolicyBundleUsesCanonicalFamilyAndFailsClosed(t *testing.T) {
	if err := EvaluatePolicyBundle(&v1alpha2.PolicyBundleSpec{
		AllowedFamilies: []v1alpha2.ArtifactFamily{v1alpha2.FamilyCapability},
	}, v1alpha2.KindBaseModel); !IsCode(err, ErrProfileFamilyDenied) {
		t.Fatalf("expected family denial, got %v", err)
	}
	if err := EvaluatePolicyBundle(&v1alpha2.PolicyBundleSpec{
		RequiredPredicates: []string{"urn:test:provenance"},
	}, v1alpha2.KindBaseModel); !IsCode(err, ErrProfilePredicateDenied) {
		t.Fatalf("expected predicate denial, got %v", err)
	}
	if err := EvaluatePolicyBundle(&v1alpha2.PolicyBundleSpec{
		MaxRiskBand: v1alpha2.RVBandGreen,
	}, v1alpha2.KindBaseModel); !IsCode(err, ErrRVRedBandBlocked) {
		t.Fatalf("expected risk denial, got %v", err)
	}
}

func TestEvaluatePolicyRejectsUnknownConfiguredRiskBand(t *testing.T) {
	result := EvaluatePolicy(&v1alpha2.PolicyBundleSpec{MaxRiskBand: v1alpha2.RVBandGreen}, v1alpha2.KindBaseModel, v1alpha2.FamilyModel, nil, "UNSET")
	if result.Allowed || result.DenyCode != ErrRVRedBandBlocked {
		t.Fatalf("got allowed=%t code=%q, want risk denial", result.Allowed, result.DenyCode)
	}
}

func TestEvaluatePolicyAllowsAndDeniesArtifactKinds(t *testing.T) {
	base := v1alpha2.KindBaseModel
	allowed := EvaluatePolicy(&v1alpha2.PolicyBundleSpec{
		AllowedArtifactTypes: []v1alpha2.ArtifactKind{base},
	}, base, v1alpha2.FamilyModel, nil, v1alpha2.RVBandGreen)
	if !allowed.Allowed {
		t.Fatalf("expected allowed artifact kind, got %q", allowed.DenyCode)
	}
	forbidden := EvaluatePolicy(&v1alpha2.PolicyBundleSpec{
		ForbiddenArtifactTypes: []v1alpha2.ArtifactKind{base},
	}, base, v1alpha2.FamilyModel, nil, v1alpha2.RVBandGreen)
	if forbidden.Allowed || forbidden.DenyCode != ErrProfileFamilyDenied {
		t.Fatalf("expected forbidden artifact denial, got allowed=%t code=%q", forbidden.Allowed, forbidden.DenyCode)
	}
	if err := EvaluatePolicyBundle(&v1alpha2.PolicyBundleSpec{
		AllowedArtifactTypes: []v1alpha2.ArtifactKind{base},
	}, base); err != nil {
		t.Fatalf("expected policy bundle to allow kind, got %v", err)
	}
	denied := EvaluatePolicy(&v1alpha2.PolicyBundleSpec{
		AllowedArtifactTypes: []v1alpha2.ArtifactKind{v1alpha2.KindLoRA},
	}, base, v1alpha2.FamilyModel, nil, v1alpha2.RVBandGreen)
	if denied.Allowed || denied.DenyCode != ErrProfileFamilyDenied {
		t.Fatalf("expected kind allowlist denial, got allowed=%t code=%q", denied.Allowed, denied.DenyCode)
	}
	if err := EvaluatePolicyBundle(&v1alpha2.PolicyBundleSpec{}, "UnknownKind"); !IsCode(err, ErrKindUnknown) {
		t.Fatalf("expected unknown kind denial, got %v", err)
	}
}
