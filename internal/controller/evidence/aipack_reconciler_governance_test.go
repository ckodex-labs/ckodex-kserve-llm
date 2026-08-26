package evidence

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func aipackWithAttestation(name string, kind servingv1alpha2.ArtifactKind, predicates ...string) servingv1alpha2.AIPack {
	entries := make([]servingv1alpha2.PredicateEntry, 0, len(predicates))
	for _, predicate := range predicates {
		entries = append(entries, servingv1alpha2.PredicateEntry{PredicateURI: predicate})
	}
	pack := *basePack(name)
	pack.Spec.Kind = kind
	pack.Spec.Source.Ref = "registry.example.test/" + name + "@sha256:" + "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	pack.Spec.Attestation = &servingv1alpha2.AIPackAttestation{Predicates: entries}
	return pack
}

func TestReconcileAIPacks_StatusBranches(t *testing.T) {
	completeAgent := aipackWithAttestation("agent", servingv1alpha2.KindAgent,
		servingv1alpha2.PredSLSAProvenance, servingv1alpha2.PredCycloneDXBOM,
		servingv1alpha2.PredAgentComposition, servingv1alpha2.PredAgentBehavioralEval,
		servingv1alpha2.PredAgentPolicyCompliance)
	incompleteAgent := *basePack("incomplete")
	incompleteAgent.Spec.Source.Ref = "registry.example.test/incomplete@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	unknownKind := *basePack("unknown")
	unknownKind.Spec.Kind = servingv1alpha2.ArtifactKind("Unknown")
	unknownKind.Spec.Source.Ref = "registry.example.test/unknown@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	tests := []struct {
		name  string
		packs []servingv1alpha2.AIPack
		want  string
	}{
		{name: "none", want: "NoAIPacksAssociated"},
		{name: "kind without required predicates", packs: []servingv1alpha2.AIPack{unknownKind}, want: "AllAIPacksAttested"},
		{name: "missing required", packs: []servingv1alpha2.AIPack{incompleteAgent}, want: "AIPackAttestationIncomplete"},
		{name: "present but cryptographically unverified", packs: []servingv1alpha2.AIPack{completeAgent}, want: "AIPackAttestationIncomplete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := baseLLMSvcForEvidence("aipack-status")
			if err := (&GovernanceReconciler{}).ReconcileAIPacks(context.Background(), svc, tt.packs); err != nil {
				t.Fatalf("ReconcileAIPacks: %v", err)
			}
			if got := condition(t, svc, "Compliance-SR-2-AIPack"); got.Reason != tt.want {
				t.Fatalf("reason = %q, want %q", got.Reason, tt.want)
			}
		})
	}
}

func TestReconcileAIPacks_AgentCompositionCreatesAdapters(t *testing.T) {
	pack := aipackWithAttestation("composed", servingv1alpha2.KindAgent,
		servingv1alpha2.PredSLSAProvenance, servingv1alpha2.PredCycloneDXBOM,
		servingv1alpha2.PredAgentComposition, servingv1alpha2.PredAgentBehavioralEval,
		servingv1alpha2.PredAgentPolicyCompliance)
	pack.Spec.Composition = &servingv1alpha2.AIPackComposition{Adapters: []servingv1alpha2.AIPackRef{{Ref: "registry.example.test/lora@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}}}
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := (&GovernanceReconciler{Client: c, Scheme: scheme}).ReconcileAIPacks(context.Background(), baseLLMSvcForEvidence("target"), []servingv1alpha2.AIPack{pack}); err != nil {
		t.Fatalf("ReconcileAIPacks: %v", err)
	}
	var adapter servingv1alpha2.LLMLoraAdapter
	if err := c.Get(context.Background(), clientKey("composed-lora-0"), &adapter); err != nil {
		t.Fatalf("created adapter: %v", err)
	}
	if adapter.Spec.TargetService != "target" {
		t.Fatalf("TargetService = %q, want target", adapter.Spec.TargetService)
	}
}

func clientKey(name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: "default"}
}
