/*
Copyright 2026 CKodex Authors.
*/

package governance

import (
	"strings"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// HasAssertedSupplyChainEvidence reports whether an adapter has a complete
// evidence bundle attached. This is enough to treat provenance as asserted,
// but not enough to claim cryptographic verification.
func HasAssertedSupplyChainEvidence(adapter *servingv1alpha2.LLMLoraAdapter) bool {
	return adapter.Status.EvidenceBundle.SignatureDigest != "" &&
		adapter.Status.EvidenceBundle.AttestationURI != "" &&
		adapter.Status.EvidenceBundle.SBOMDigest != ""
}

// HasVerifiedSupplyChainEvidence reports whether an adapter has a complete
// evidence bundle plus a non-placeholder attestation record. Internal
// ckodex:// receipts remain asserted-only until a cryptographic verifier
// writes an externally verifiable attestation reference.
func HasVerifiedSupplyChainEvidence(adapter *servingv1alpha2.LLMLoraAdapter) bool {
	return HasAssertedSupplyChainEvidence(adapter) &&
		adapter.Status.EvidenceBundle.LastVerifiedAt != nil &&
		!strings.HasPrefix(adapter.Status.EvidenceBundle.AttestationURI, "ckodex://")
}
