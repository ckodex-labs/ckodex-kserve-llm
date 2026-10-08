/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// WorkloadClass defines the workload category served by a model service.
// +kubebuilder:validation:Enum=generative;embedding;reranking;evaluation;decision
type WorkloadClass string

const (
	WorkloadClassGenerative WorkloadClass = "generative"
	WorkloadClassEmbedding  WorkloadClass = "embedding"
	WorkloadClassReranking  WorkloadClass = "reranking"
	WorkloadClassEvaluation WorkloadClass = "evaluation"
	WorkloadClassDecision   WorkloadClass = "decision"
)

// DecisionCapability defines the decision primitive evaluated by a decision workload.
// +kubebuilder:validation:Enum=predicate;choice;score
type DecisionCapability string

const (
	DecisionCapabilityPredicate DecisionCapability = "predicate"
	DecisionCapabilityChoice    DecisionCapability = "choice"
	DecisionCapabilityScore     DecisionCapability = "score"
)

// SubstrateVectorState represents discrete qualitative states for each substrate dimension.
// Vector state, not booleans.
type SubstrateVectorState string

const (
	VectorStateUnknown   SubstrateVectorState = "UNKNOWN"
	VectorStatePending   SubstrateVectorState = "PENDING"
	VectorStateSatisfied SubstrateVectorState = "SATISFIED"
	VectorStateDegraded  SubstrateVectorState = "DEGRADED"
	VectorStateFailed    SubstrateVectorState = "FAILED"
)

// Substrate dimension constants.
const (
	SubstrateDimensionReady      = "READY"
	SubstrateDimensionAdmitted   = "ADMITTED"
	SubstrateDimensionCalibrated = "CALIBRATED"
	SubstrateDimensionCompatible = "COMPATIBLE"
	SubstrateDimensionResident   = "RESIDENT"
	SubstrateDimensionAvailable  = "AVAILABLE"
)

// EvaluatorSubstrateVector represents the 6-dimensional execution substrate state for evaluators.
// The operator governs this substrate vector and does NOT make semantic domain decisions.
type EvaluatorSubstrateVector struct {
	// Ready indicates the execution endpoint/substrate is initialized and ready.
	Ready SubstrateVectorState `json:"ready"`
	// Admitted indicates the workload meets admission policy and governance prerequisites.
	Admitted SubstrateVectorState `json:"admitted"`
	// Calibrated indicates operational calibration, baselines, and scoring limits are verified.
	Calibrated SubstrateVectorState `json:"calibrated"`
	// Compatible indicates workload class, decision capabilities, and runtime engines match.
	Compatible SubstrateVectorState `json:"compatible"`
	// Resident indicates weights, adapters, or model artifacts are loaded in memory/cache.
	Resident SubstrateVectorState `json:"resident"`
	// Available indicates capacity, SLA, and concurrency targets are met.
	Available SubstrateVectorState `json:"available"`
}

// ProfileSLA defines service level agreements for the workload profile.
type ProfileSLA struct {
	// MaxLatencyMillis specifies the 99th percentile latency target in milliseconds.
	// +optional
	MaxLatencyMillis *int32 `json:"maxLatencyMillis,omitempty"`
	// TargetAvailability is the required uptime percentage string (e.g. "99.9%").
	// +optional
	TargetAvailability string `json:"targetAvailability,omitempty"`
	// TargetThroughputRPS is the required throughput in requests per second.
	// +optional
	TargetThroughputRPS *int32 `json:"targetThroughputRPS,omitempty"`
	// TimeoutSeconds specifies request timeout limit.
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// HardwareRequirementSpec describes minimum hardware requirements.
type HardwareRequirementSpec struct {
	// Accelerator defines the required accelerator type (e.g., "nvidia-h100", "nvidia-a100", "cpu").
	// +optional
	Accelerator string `json:"accelerator,omitempty"`
	// MinMemory specifies the minimum memory string (e.g., "16Gi").
	// +optional
	MinMemory string `json:"minMemory,omitempty"`
	// MinGPUCount specifies the minimum number of GPUs required.
	// +optional
	MinGPUCount *int32 `json:"minGPUCount,omitempty"`
}

// ProfileApplicability defines where and when this serving profile applies.
type ProfileApplicability struct {
	// SupportedRuntimes lists the execution runtimes compatible with this profile.
	// +optional
	SupportedRuntimes []string `json:"supportedRuntimes,omitempty"`
	// TargetEngines defines the specific serving engine implementations.
	// +optional
	TargetEngines []string `json:"targetEngines,omitempty"`
	// ModelFamilies specifies the model architecture families supported.
	// +optional
	ModelFamilies []string `json:"modelFamilies,omitempty"`
	// WorkloadSelector selects target workloads by label.
	// +optional
	WorkloadSelector map[string]string `json:"workloadSelector,omitempty"`
	// HardwareRequirements describes minimum hardware needed.
	// +optional
	HardwareRequirements *HardwareRequirementSpec `json:"hardwareRequirements,omitempty"`
}

// ProfileEvidence defines governance and supply-chain evidence requirements.
type ProfileEvidence struct {
	// RequiredAttestations lists predicate types or URIs required before admission.
	// +optional
	RequiredAttestations []string `json:"requiredAttestations,omitempty"`
	// MinTrustLevel specifies minimum trust level: denied, unknown, asserted, verified, trusted.
	// +optional
	MinTrustLevel string `json:"minTrustLevel,omitempty"`
	// RequireSBOM specifies whether an SBOM digest must be attached.
	// +optional
	RequireSBOM bool `json:"requireSBOM,omitempty"`
	// RequireSignature specifies whether a cryptographic signature is mandatory.
	// +optional
	RequireSignature bool `json:"requireSignature,omitempty"`
}

// RuntimeLinkSpec specifies how the profile links to execution runtimes.
type RuntimeLinkSpec struct {
	// Engine identifies the execution substrate runtime (e.g., "vllm", "triton", "eval-runner", "custom").
	Engine string `json:"engine"`
	// Image specifies the container image implementing the evaluator or serving runtime.
	// +optional
	Image string `json:"image,omitempty"`
	// Endpoint specifies a pre-existing runtime endpoint URL or service name.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Port defines the network port exposed by the runtime substrate.
	// +optional
	Port int32 `json:"port,omitempty"`
	// Protocol specifies the communication protocol (e.g., "http", "grpc", "v2-dataplane").
	// +optional
	Protocol string `json:"protocol,omitempty"`
	// ServiceAccount specifies the runtime identity.
	// +optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

// ModelServingProfileSpec defines the desired state of ModelServingProfile
type ModelServingProfileSpec struct {
	// WorkloadClass specifies the category of workload (generative, embedding, reranking, evaluation, decision).
	WorkloadClass WorkloadClass `json:"workloadClass"`
	// DecisionCapabilities specifies the decision primitives supported (predicate, choice, score).
	// Applicable when WorkloadClass is decision or evaluation.
	// +optional
	DecisionCapabilities []DecisionCapability `json:"decisionCapabilities,omitempty"`
	// Runtime specifies how the profile links to execution runtimes.
	// +optional
	Runtime *RuntimeLinkSpec `json:"runtime,omitempty"`
	// SLA specifies service level agreements for this profile.
	// +optional
	SLA *ProfileSLA `json:"sla,omitempty"`
	// Applicability specifies workload and engine matching rules.
	// +optional
	Applicability *ProfileApplicability `json:"applicability,omitempty"`
	// Evidence specifies governance and verification prerequisites.
	// +optional
	Evidence *ProfileEvidence `json:"evidence,omitempty"`
}

// ModelServingProfileStatus defines the observed state of ModelServingProfile
type ModelServingProfileStatus struct {
	// SubstrateVector captures the 6-plane substrate readiness vector.
	// +optional
	SubstrateVector EvaluatorSubstrateVector `json:"substrateVector,omitempty"`
	// StatePlanes provides orthogonal lifecycle/trust/risk planes.
	// +optional
	StatePlanes StatePlanes `json:"statePlanes,omitempty"`
	// Conditions represent the latest available observations of current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ActiveEvaluators is the count of active evaluators using this profile.
	// +optional
	ActiveEvaluators int32 `json:"activeEvaluators,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=msp
// +kubebuilder:printcolumn:name="WorkloadClass",type="string",JSONPath=".spec.workloadClass"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.substrateVector.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ModelServingProfile defines workload characteristics, SLA, applicability, and evidence constraints
// for model serving and decision evaluator workloads.
type ModelServingProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelServingProfileSpec   `json:"spec,omitempty"`
	Status ModelServingProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelServingProfileList contains a list of ModelServingProfile
type ModelServingProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelServingProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelServingProfile{}, &ModelServingProfileList{})
}

// DeepCopyInto copies receiver into out.
func (in *ModelServingProfile) DeepCopyInto(out *ModelServingProfile) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy copies receiver, creating a new ModelServingProfile.
func (in *ModelServingProfile) DeepCopy() *ModelServingProfile {
	if in == nil {
		return nil
	}
	out := new(ModelServingProfile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a generically typed copy of an object.
func (in *ModelServingProfile) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies receiver into out.
func (in *ModelServingProfileList) DeepCopyInto(out *ModelServingProfileList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ModelServingProfile, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy copies receiver, creating a new ModelServingProfileList.
func (in *ModelServingProfileList) DeepCopy() *ModelServingProfileList {
	if in == nil {
		return nil
	}
	out := new(ModelServingProfileList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a generically typed copy of an object.
func (in *ModelServingProfileList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies receiver into out.
func (in *ModelServingProfileSpec) DeepCopyInto(out *ModelServingProfileSpec) {
	*out = *in
	if in.DecisionCapabilities != nil {
		out.DecisionCapabilities = make([]DecisionCapability, len(in.DecisionCapabilities))
		copy(out.DecisionCapabilities, in.DecisionCapabilities)
	}
	if in.Runtime != nil {
		out.Runtime = in.Runtime.DeepCopy()
	}
	if in.SLA != nil {
		out.SLA = in.SLA.DeepCopy()
	}
	if in.Applicability != nil {
		out.Applicability = in.Applicability.DeepCopy()
	}
	if in.Evidence != nil {
		out.Evidence = in.Evidence.DeepCopy()
	}
}

// DeepCopy copies receiver, creating a new ModelServingProfileSpec.
func (in *ModelServingProfileSpec) DeepCopy() *ModelServingProfileSpec {
	if in == nil {
		return nil
	}
	out := new(ModelServingProfileSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *ModelServingProfileStatus) DeepCopyInto(out *ModelServingProfileStatus) {
	*out = *in
	out.SubstrateVector = in.SubstrateVector
	out.StatePlanes = in.StatePlanes
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy copies receiver, creating a new ModelServingProfileStatus.
func (in *ModelServingProfileStatus) DeepCopy() *ModelServingProfileStatus {
	if in == nil {
		return nil
	}
	out := new(ModelServingProfileStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *ProfileSLA) DeepCopyInto(out *ProfileSLA) {
	*out = *in
	if in.MaxLatencyMillis != nil {
		in, out := &in.MaxLatencyMillis, &out.MaxLatencyMillis
		*out = new(int32)
		**out = **in
	}
	if in.TargetThroughputRPS != nil {
		in, out := &in.TargetThroughputRPS, &out.TargetThroughputRPS
		*out = new(int32)
		**out = **in
	}
	if in.TimeoutSeconds != nil {
		in, out := &in.TimeoutSeconds, &out.TimeoutSeconds
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy copies receiver, creating a new ProfileSLA.
func (in *ProfileSLA) DeepCopy() *ProfileSLA {
	if in == nil {
		return nil
	}
	out := new(ProfileSLA)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *HardwareRequirementSpec) DeepCopyInto(out *HardwareRequirementSpec) {
	*out = *in
	if in.MinGPUCount != nil {
		in, out := &in.MinGPUCount, &out.MinGPUCount
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy copies receiver, creating a new HardwareRequirementSpec.
func (in *HardwareRequirementSpec) DeepCopy() *HardwareRequirementSpec {
	if in == nil {
		return nil
	}
	out := new(HardwareRequirementSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *ProfileApplicability) DeepCopyInto(out *ProfileApplicability) {
	*out = *in
	if in.SupportedRuntimes != nil {
		out.SupportedRuntimes = make([]string, len(in.SupportedRuntimes))
		copy(out.SupportedRuntimes, in.SupportedRuntimes)
	}
	if in.TargetEngines != nil {
		out.TargetEngines = make([]string, len(in.TargetEngines))
		copy(out.TargetEngines, in.TargetEngines)
	}
	if in.ModelFamilies != nil {
		out.ModelFamilies = make([]string, len(in.ModelFamilies))
		copy(out.ModelFamilies, in.ModelFamilies)
	}
	if in.WorkloadSelector != nil {
		out.WorkloadSelector = make(map[string]string, len(in.WorkloadSelector))
		for k, v := range in.WorkloadSelector {
			out.WorkloadSelector[k] = v
		}
	}
	if in.HardwareRequirements != nil {
		out.HardwareRequirements = in.HardwareRequirements.DeepCopy()
	}
}

// DeepCopy copies receiver, creating a new ProfileApplicability.
func (in *ProfileApplicability) DeepCopy() *ProfileApplicability {
	if in == nil {
		return nil
	}
	out := new(ProfileApplicability)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *ProfileEvidence) DeepCopyInto(out *ProfileEvidence) {
	*out = *in
	if in.RequiredAttestations != nil {
		out.RequiredAttestations = make([]string, len(in.RequiredAttestations))
		copy(out.RequiredAttestations, in.RequiredAttestations)
	}
}

// DeepCopy copies receiver, creating a new ProfileEvidence.
func (in *ProfileEvidence) DeepCopy() *ProfileEvidence {
	if in == nil {
		return nil
	}
	out := new(ProfileEvidence)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies receiver into out.
func (in *RuntimeLinkSpec) DeepCopyInto(out *RuntimeLinkSpec) {
	*out = *in
}

// DeepCopy copies receiver, creating a new RuntimeLinkSpec.
func (in *RuntimeLinkSpec) DeepCopy() *RuntimeLinkSpec {
	if in == nil {
		return nil
	}
	out := new(RuntimeLinkSpec)
	in.DeepCopyInto(out)
	return out
}
