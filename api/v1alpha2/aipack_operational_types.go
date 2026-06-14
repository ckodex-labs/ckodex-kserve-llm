/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package v1alpha2 — operational types for AIPACK-SPEC v0.1.1 §§11-22.
package v1alpha2

// AIPackLineageEnvelope is a provenance attribution record per §11.
type AIPackLineageEnvelope struct {
	// SourceRef is the OCI digest ref of the upstream artifact this was derived from.
	SourceRef string `json:"sourceRef,omitempty"`
	// Parents lists direct parent artifacts in the derivation graph.
	// +optional
	Parents []string `json:"parents,omitempty"`
}

// DeprecationPhase is the AIPACK artifact deprecation lifecycle phase per §16.
type DeprecationPhase string

const (
	DeprecationPhaseDeprecated DeprecationPhase = "Deprecated"
	DeprecationPhaseEndOfLife  DeprecationPhase = "EndOfLife"
)

// AIPackDeprecationNotice describes the deprecation state of an artifact per §16.
type AIPackDeprecationNotice struct {
	// Phase is the deprecation phase.
	Phase DeprecationPhase `json:"phase"`
	// Reason is a human-readable explanation.
	Reason string `json:"reason,omitempty"`
	// SunsetDate is the ISO8601 date after which the artifact is no longer usable (§16 EndOfLife gate).
	// +optional
	SunsetDate string `json:"sunsetDate,omitempty"`
	// DerogationRef references a signed profile derogation that extends the sunset window.
	// +optional
	DerogationRef string `json:"derogationRef,omitempty"`
}

// AIPackAirGapBundle describes an offline bundle per §17.
type AIPackAirGapBundle struct {
	// TrustRootRef is the OCI digest ref of the embedded CA bundle.
	TrustRootRef string `json:"trustRootRef,omitempty"`
	// TSACertRef is the OCI digest ref of the offline TSA certificate.
	TSACertRef string `json:"tsaCertRef,omitempty"`
	// BundleRef is the OCI digest ref of the air-gap bundle artifact.
	// +optional
	BundleRef string `json:"bundleRef,omitempty"`
}

// AIPackQuarantineTrigger describes a quarantine trigger per §21.
type AIPackQuarantineTrigger struct {
	// Fired indicates the trigger condition has been met.
	Fired bool `json:"fired"`
	// Reason is the human-readable trigger reason (required when Fired=true).
	// +optional
	Reason string `json:"reason,omitempty"`
	// EscalationFail indicates that the quarantine escalation protocol failed.
	// +optional
	EscalationFail bool `json:"escalationFail,omitempty"`
}

// AIPackOutlierSignal describes an outlier signal per §14.
type AIPackOutlierSignal struct {
	// Category classifies the outlier (e.g., "statistical-outlier", "behavioral-anomaly").
	Category string `json:"category"`
	// Acknowledged indicates the signal has been reviewed and accepted.
	Acknowledged bool `json:"acknowledged"`
	// AcknowledgedBy identifies the reviewer who accepted this signal.
	// +optional
	AcknowledgedBy string `json:"acknowledgedBy,omitempty"`
}

// AIPackPolicySpec is an alias for PolicyBundleSpec for use in operational validation APIs.
// Use PolicyBundleSpec for CRD fields; AIPackPolicySpec in test and operational code.
type AIPackPolicySpec = PolicyBundleSpec
