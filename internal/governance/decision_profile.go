/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package governance

import (
	"context"
	"errors"
	"fmt"
	"time"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrOperatorNotSemanticAuthority is returned when a caller attempts to delegate
// domain semantic judgments to the operator. The operator only manages substrate execution.
var ErrOperatorNotSemanticAuthority = errors.New(
	"operator provides execution substrate only and does not make semantic domain decisions",
)

// DimensionObservation captures the vector state and diagnostic reason for a single substrate dimension.
type DimensionObservation struct {
	Dimension string
	State     servingv1alpha2.SubstrateVectorState
	Reason    string
	Message   string
}

// EvaluatorTarget represents a workload target running on the operator's execution substrate.
type EvaluatorTarget struct {
	Name                 string
	Namespace            string
	WorkloadClass        servingv1alpha2.WorkloadClass
	Capabilities         []servingv1alpha2.DecisionCapability
	RuntimeEngine        string
	Endpoint             string
	EndpointHealthy      bool
	EndpointInitializing bool
	TrustLevel           string
	Attestations         []string
	HasSignature         bool
	HasSBOM              bool
	ArtifactsResident    bool
	ArtifactsLoading     bool
	CalibrationScore     *float64
	HasCalibration       bool
	ObservedLatencyMs    int32
	ReadyReplicas        int32
	DesiredReplicas      int32
}

// SubstrateEvaluationReport aggregates vector state across all 6 substrate dimensions.
type SubstrateEvaluationReport struct {
	WorkloadName   string
	ProfileName    string
	Vector         servingv1alpha2.EvaluatorSubstrateVector
	Observations   map[string]DimensionObservation
	IsOperable     bool
	FailureReasons []string
	EvaluatedAt    time.Time
}

// ValidateModelServingProfile validates the structural integrity of a ModelServingProfile spec.
func ValidateModelServingProfile(profile *servingv1alpha2.ModelServingProfile) error {
	if profile == nil {
		return errors.New("profile cannot be nil")
	}
	spec := &profile.Spec
	switch spec.WorkloadClass {
	case servingv1alpha2.WorkloadClassGenerative,
		servingv1alpha2.WorkloadClassEmbedding,
		servingv1alpha2.WorkloadClassReranking,
		servingv1alpha2.WorkloadClassEvaluation,
		servingv1alpha2.WorkloadClassDecision:
	default:
		return fmt.Errorf("invalid workloadClass %q", spec.WorkloadClass)
	}

	if spec.WorkloadClass == servingv1alpha2.WorkloadClassDecision {
		if len(spec.DecisionCapabilities) == 0 {
			return errors.New("decision workload class requires at least one decision capability")
		}
	}

	for _, cap := range spec.DecisionCapabilities {
		switch cap {
		case servingv1alpha2.DecisionCapabilityPredicate,
			servingv1alpha2.DecisionCapabilityChoice,
			servingv1alpha2.DecisionCapabilityScore:
		default:
			return fmt.Errorf("invalid decisionCapability %q", cap)
		}
	}
	return nil
}

// ExecuteDomainDecision explicitly enforces that the operator refuses to execute semantic domain decisions.
func ExecuteDomainDecision(_ context.Context, _ *EvaluatorTarget, _ any) error {
	return fmt.Errorf("refusing semantic decision execution: %w", ErrOperatorNotSemanticAuthority)
}

// EvaluateSubstrateState performs a multi-dimensional check of the execution substrate.
func EvaluateSubstrateState(
	_ context.Context,
	profile *servingv1alpha2.ModelServingProfile,
	target *EvaluatorTarget,
) (*SubstrateEvaluationReport, error) {
	if profile == nil || target == nil {
		return nil, errors.New("profile and target must not be nil")
	}
	if err := ValidateModelServingProfile(profile); err != nil {
		return nil, fmt.Errorf("invalid profile spec: %w", err)
	}

	obsMap := make(map[string]DimensionObservation)
	obsMap[servingv1alpha2.SubstrateDimensionReady] = checkReady(target)
	obsMap[servingv1alpha2.SubstrateDimensionAdmitted] = checkAdmitted(target, profile)
	obsMap[servingv1alpha2.SubstrateDimensionCalibrated] = checkCalibrated(target, profile)
	obsMap[servingv1alpha2.SubstrateDimensionCompatible] = checkCompatible(target, profile)
	obsMap[servingv1alpha2.SubstrateDimensionResident] = checkResident(target)
	obsMap[servingv1alpha2.SubstrateDimensionAvailable] = checkAvailable(target, profile)

	vector := servingv1alpha2.EvaluatorSubstrateVector{
		Ready:      obsMap[servingv1alpha2.SubstrateDimensionReady].State,
		Admitted:   obsMap[servingv1alpha2.SubstrateDimensionAdmitted].State,
		Calibrated: obsMap[servingv1alpha2.SubstrateDimensionCalibrated].State,
		Compatible: obsMap[servingv1alpha2.SubstrateDimensionCompatible].State,
		Resident:   obsMap[servingv1alpha2.SubstrateDimensionResident].State,
		Available:  obsMap[servingv1alpha2.SubstrateDimensionAvailable].State,
	}

	failures := collectFailures(obsMap)
	isOperable := len(failures) == 0 && vector.Ready == servingv1alpha2.VectorStateSatisfied

	return &SubstrateEvaluationReport{
		WorkloadName:   target.Name,
		ProfileName:    profile.Name,
		Vector:         vector,
		Observations:   obsMap,
		IsOperable:     isOperable,
		FailureReasons: failures,
		EvaluatedAt:    time.Now(),
	}, nil
}

func checkReady(target *EvaluatorTarget) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionReady
	if target.EndpointInitializing {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStatePending, Reason: "Initializing"}
	}
	if target.Endpoint == "" || !target.EndpointHealthy {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "EndpointUnhealthy"}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "EndpointReady"}
}

func checkAdmitted(target *EvaluatorTarget, profile *servingv1alpha2.ModelServingProfile) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionAdmitted
	ev := profile.Spec.Evidence
	if ev == nil {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "NoEvidenceConstraints"}
	}
	if target.TrustLevel == "denied" {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "TrustDenied"}
	}
	if ev.MinTrustLevel != "" && !isTrustSufficient(target.TrustLevel, ev.MinTrustLevel) {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "InsufficientTrust"}
	}
	for _, reqAttest := range ev.RequiredAttestations {
		if !containsString(target.Attestations, reqAttest) {
			return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "MissingAttestation"}
		}
	}
	if ev.RequireSBOM && !target.HasSBOM {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "MissingSBOM"}
	}
	if ev.RequireSignature && !target.HasSignature {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "MissingSignature"}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "Admitted"}
}

func checkCalibrated(target *EvaluatorTarget, profile *servingv1alpha2.ModelServingProfile) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionCalibrated
	isDecisionClass := profile.Spec.WorkloadClass == servingv1alpha2.WorkloadClassDecision ||
		profile.Spec.WorkloadClass == servingv1alpha2.WorkloadClassEvaluation

	if !isDecisionClass {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "CalibrationNotRequired"}
	}
	if !target.HasCalibration {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "UncalibratedEvaluator"}
	}
	if target.CalibrationScore != nil && *target.CalibrationScore < 0 {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateDegraded, Reason: "CalibrationDegraded"}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "Calibrated"}
}

func checkCompatible(target *EvaluatorTarget, profile *servingv1alpha2.ModelServingProfile) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionCompatible
	if target.WorkloadClass != profile.Spec.WorkloadClass {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "WorkloadClassMismatch"}
	}
	for _, reqCap := range profile.Spec.DecisionCapabilities {
		if !containsCapability(target.Capabilities, reqCap) {
			return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "MissingDecisionCapability"}
		}
	}
	if profile.Spec.Runtime != nil && profile.Spec.Runtime.Engine != "" {
		if target.RuntimeEngine != "" && target.RuntimeEngine != profile.Spec.Runtime.Engine {
			return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "IncompatibleRuntimeEngine"}
		}
	}
	if profile.Spec.Applicability != nil && len(profile.Spec.Applicability.SupportedRuntimes) > 0 {
		if !containsString(profile.Spec.Applicability.SupportedRuntimes, target.RuntimeEngine) {
			return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "UnsupportedRuntime"}
		}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "Compatible"}
}

func checkResident(target *EvaluatorTarget) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionResident
	if target.ArtifactsLoading {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStatePending, Reason: "WarmingCache"}
	}
	if !target.ArtifactsResident {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "ArtifactsNotResident"}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "Resident"}
}

func checkAvailable(target *EvaluatorTarget, profile *servingv1alpha2.ModelServingProfile) DimensionObservation {
	dim := servingv1alpha2.SubstrateDimensionAvailable
	if target.ReadyReplicas <= 0 {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateFailed, Reason: "ZeroReadyReplicas"}
	}
	if target.DesiredReplicas > 0 && target.ReadyReplicas < target.DesiredReplicas {
		return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateDegraded, Reason: "ReplicasDegraded"}
	}
	if profile.Spec.SLA != nil && profile.Spec.SLA.MaxLatencyMillis != nil {
		if target.ObservedLatencyMs > *profile.Spec.SLA.MaxLatencyMillis {
			return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateDegraded, Reason: "LatencySLAViolated"}
		}
	}
	return DimensionObservation{Dimension: dim, State: servingv1alpha2.VectorStateSatisfied, Reason: "Available"}
}

func collectFailures(obsMap map[string]DimensionObservation) []string {
	var failures []string
	for dim, obs := range obsMap {
		if obs.State == servingv1alpha2.VectorStateFailed || obs.State == servingv1alpha2.VectorStatePending {
			failures = append(failures, fmt.Sprintf("%s:%s", dim, obs.Reason))
		}
	}
	return failures
}

func isTrustSufficient(actual, required string) bool {
	trustScores := map[string]int{
		"denied":   -1,
		"unknown":  0,
		"asserted": 1,
		"verified": 2,
		"trusted":  3,
	}
	return trustScores[actual] >= trustScores[required]
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func containsCapability(slice []servingv1alpha2.DecisionCapability, cap servingv1alpha2.DecisionCapability) bool {
	for _, item := range slice {
		if item == cap {
			return true
		}
	}
	return false
}

// ApplySubstrateStatus updates the ModelServingProfile status from an evaluation report.
func ApplySubstrateStatus(status *servingv1alpha2.ModelServingProfileStatus, report *SubstrateEvaluationReport) {
	if status == nil || report == nil {
		return
	}
	status.SubstrateVector = report.Vector
	readyCondStatus := metav1.ConditionFalse
	readyReason := "SubstrateFailed"
	if report.IsOperable {
		readyCondStatus = metav1.ConditionTrue
		readyReason = "SubstrateOperable"
	}
	now := metav1.NewTime(report.EvaluatedAt)
	status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             readyCondStatus,
			LastTransitionTime: now,
			Reason:             readyReason,
			Message:            fmt.Sprintf("Operable: %t, failures: %v", report.IsOperable, report.FailureReasons),
		},
	}
}
