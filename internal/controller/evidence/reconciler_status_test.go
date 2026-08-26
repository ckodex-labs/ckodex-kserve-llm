package evidence

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func condition(t *testing.T, svc *servingv1alpha2.LLMInferenceService, typ string) metav1.Condition {
	t.Helper()
	for _, c := range svc.Status.Conditions {
		if c.Type == typ {
			return c
		}
	}
	t.Fatalf("condition %s not found", typ)
	return metav1.Condition{}
}

func verifiedAdapter() servingv1alpha2.LLMLoraAdapter {
	now := metav1.Now()
	return servingv1alpha2.LLMLoraAdapter{Spec: servingv1alpha2.LLMLoraAdapterSpec{
		ToolSurface: &servingv1alpha2.ToolSurface{AllowedAPIs: []string{"api.example.test"}},
	}, Status: servingv1alpha2.LLMLoraAdapterStatus{
		StatePlanes: servingv1alpha2.StatePlanes{Lifecycle: "active", Trust: "verified", Risk: "low"},
		EvidenceBundle: servingv1alpha2.EvidenceBundle{
			SignatureDigest: "sha256:signature", AttestationURI: "https://example.test/attestation",
			SBOMDigest: "sha256:sbom", LastVerifiedAt: &now,
		},
	}}
}

func TestReconcileStatus_FailClosedAndOfflineKeyBranches(t *testing.T) {
	tests := []struct {
		name, key, reason string
	}{
		{name: "missing offline key", reason: "OfflineKeyMissing"},
		{name: "configured offline key", key: "/keys/cosign.pub", reason: "OfflineVerificationPending"},
		{name: "no verifiable artifacts", reason: "NoVerifiableArtifacts", key: "ignored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := baseLLMSvcForEvidence("status-" + tt.name)
			if tt.name == "no verifiable artifacts" {
				// This case is intentionally online; the key value is not used.
				tt.key = ""
			}
			r := &GovernanceReconciler{AirGappedMode: tt.name != "no verifiable artifacts", LocalCosignKeyPath: tt.key}
			if err := r.Reconcile(context.Background(), svc, nil); err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			got := condition(t, svc, "Compliance-SR-2")
			if got.Reason != tt.reason {
				t.Fatalf("SR-2 reason = %q, want %q", got.Reason, tt.reason)
			}
		})
	}
}

func TestReconcileStatus_VerifiedAdaptersProduceProvenance(t *testing.T) {
	svc := baseLLMSvcForEvidence("verified-adapter")
	r := &GovernanceReconciler{}
	if err := r.Reconcile(context.Background(), svc, []servingv1alpha2.LLMLoraAdapter{verifiedAdapter()}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := condition(t, svc, "Compliance-AC-4"); got.Status != metav1.ConditionUnknown || got.Reason != "EvidenceUnavailable" {
		t.Errorf("AC-4 = %#v, want unknown with unavailable evidence", got)
	}
	for _, control := range []string{"Compliance-AU-2", "Compliance-SI-4"} {
		if got := condition(t, svc, control); got.Status != metav1.ConditionUnknown || got.Reason != "EvidenceUnavailable" {
			t.Errorf("%s = %#v, want unknown with unavailable evidence", control, got)
		}
	}
	if got := condition(t, svc, "Compliance-SI-7"); got.Status != metav1.ConditionFalse || got.Reason != "IntegrityUnverified" {
		t.Errorf("SI-7 = %#v, want asserted/unverified", got)
	}
	if got := condition(t, svc, "Compliance-SR-2"); got.Status != metav1.ConditionTrue || got.Reason != "ProvenanceVerified" {
		t.Errorf("SR-2 = %#v, want provenance verified", got)
	}
}

func TestReconcileStatus_ConfiguredToolSurfaceDoesNotProveAC4(t *testing.T) {
	svc := baseLLMSvcForEvidence("tool-surface-only")
	svc.Spec.ToolSurface = &servingv1alpha2.ToolSurface{AllowedAPIs: []string{"api.example.test"}}
	if err := (&GovernanceReconciler{}).Reconcile(context.Background(), svc, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := condition(t, svc, "Compliance-AC-4"); got.Status == metav1.ConditionTrue {
		t.Fatalf("AC-4 = %#v, configured ToolSurface must not prove enforcement", got)
	}
}

func TestReconcileStatus_DeniedAdapterMarksSecurityBreach(t *testing.T) {
	adapter := verifiedAdapter()
	adapter.Status.StatePlanes.Trust = "denied"
	svc := baseLLMSvcForEvidence("denied-adapter")
	if err := (&GovernanceReconciler{}).Reconcile(context.Background(), svc, []servingv1alpha2.LLMLoraAdapter{adapter}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := condition(t, svc, "Compliance-SI-7"); got.Reason != "SecurityBreach" {
		t.Fatalf("SI-7 reason = %q, want SecurityBreach", got.Reason)
	}
}
