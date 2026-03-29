/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// CoactorPattern defines the collaboration pattern.
type CoactorPattern string

const (
	PatternPrefillDecode   CoactorPattern = "prefill-decode"
	PatternAgentEnsemble   CoactorPattern = "agent-ensemble"
	PatternPipeline        CoactorPattern = "pipeline"
	PatternMixtureOfAgents CoactorPattern = "mixture-of-agents"
)

// CoordinationMethod defines synchronization approach.
type CoordinationMethod string

const (
	CoordinationDirect   CoordinationMethod = "direct"
	CoordinationPubSub   CoordinationMethod = "pubsub"
	CoordinationWorkflow CoordinationMethod = "workflow"
)

// CoactorGroupPhase represents the group lifecycle.
type CoactorGroupPhase string

const (
	CoactorGroupForming   CoactorGroupPhase = "Forming"
	CoactorGroupReady     CoactorGroupPhase = "Ready"
	CoactorGroupDegraded  CoactorGroupPhase = "Degraded"
	CoactorGroupDissolved CoactorGroupPhase = "Dissolved"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Pattern",type=string,JSONPath=`.spec.pattern`
// +kubebuilder:printcolumn:name="Members",type=integer,JSONPath=`.status.activeMemberCount`

// CoactorGroup is the stable v1 API for collaborative actor groups.
// A group defines prefill+decode pairs, agent ensembles, or model pipelines.
type CoactorGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CoactorGroupSpec   `json:"spec,omitempty"`
	Status            CoactorGroupStatus `json:"status,omitempty"`
}

// CoactorGroupSpec defines the coactor group configuration.
type CoactorGroupSpec struct {
	// Pattern defines the collaboration pattern.
	Pattern CoactorPattern `json:"pattern"`

	// Members are the actors in this group.
	Members []CoactorMember `json:"members"`

	// Coordination defines how actors synchronize.
	Coordination CoordinationConfig `json:"coordination"`

	// SessionAffinity keeps all members colocated for a session.
	// +kubebuilder:default=true
	// +optional
	SessionAffinity bool `json:"sessionAffinity,omitempty"`
}

// CoactorMember defines a single member of the group.
type CoactorMember struct {
	// Name identifies this member within the group.
	Name string `json:"name"`

	// Role defines the member's role (e.g., "prefill", "decode", "router", "worker").
	Role string `json:"role"`

	// ActorRef references the InferenceActor.
	ActorRef string `json:"actorRef"`

	// Weight for load balancing within the group.
	// +kubebuilder:default="1"
	// +optional
	Weight string `json:"weight,omitempty"`

	// DependsOn lists other members that must be active before this one.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`
}

// CoordinationConfig defines how coactors synchronize.
type CoordinationConfig struct {
	// Method is the coordination method.
	Method CoordinationMethod `json:"method"`

	// KVTransfer configures KV-cache transfer between prefill/decode actors.
	// +optional
	KVTransfer *KVTransferConfig `json:"kvTransfer,omitempty"`

	// Aggregation configures how results from ensemble members are combined.
	// +optional
	Aggregation *AggregationConfig `json:"aggregation,omitempty"`
}

// KVTransferConfig defines KV-cache transfer settings.
type KVTransferConfig struct {
	// Method is the transfer mechanism: nccl, rdma, or tcp.
	// +kubebuilder:default="nccl"
	Method string `json:"method"`
}

// AggregationConfig defines how ensemble results are combined.
type AggregationConfig struct {
	// Strategy is the aggregation strategy: best-of-n, voting, weighted-merge, chain.
	// +kubebuilder:default="best-of-n"
	Strategy string `json:"strategy"`
}

// MemberStatus tracks an individual member's state.
type MemberStatus struct {
	Name  string     `json:"name"`
	Role  string     `json:"role"`
	State ActorState `json:"state"`
	Ready bool       `json:"ready"`
}

// CoactorGroupStatus defines the observed state.
type CoactorGroupStatus struct {
	// Phase is the group lifecycle phase.
	Phase CoactorGroupPhase `json:"phase,omitempty"`

	// ActiveMemberCount is the number of active members.
	ActiveMemberCount int32 `json:"activeMemberCount,omitempty"`

	// MemberStatuses tracks each member's state.
	// +optional
	MemberStatuses []MemberStatus `json:"memberStatuses,omitempty"`

	// Conditions track group health.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// CoactorGroupList contains a list of CoactorGroup.
type CoactorGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CoactorGroup `json:"items"`
}

// Hub marks CoactorGroup as the conversion hub (storage version).
func (*CoactorGroup) Hub() {}

// --- DeepCopy implementations ---

func (in *CoactorGroup) DeepCopyInto(out *CoactorGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *CoactorGroup) DeepCopy() *CoactorGroup {
	if in == nil {
		return nil
	}
	out := new(CoactorGroup)
	in.DeepCopyInto(out)
	return out
}

func (in *CoactorGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *CoactorGroupList) DeepCopyInto(out *CoactorGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CoactorGroup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *CoactorGroupList) DeepCopy() *CoactorGroupList {
	if in == nil {
		return nil
	}
	out := new(CoactorGroupList)
	in.DeepCopyInto(out)
	return out
}

func (in *CoactorGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *CoactorGroupSpec) DeepCopyInto(out *CoactorGroupSpec) {
	*out = *in
	if in.Members != nil {
		in, out := &in.Members, &out.Members
		*out = make([]CoactorMember, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Coordination.DeepCopyInto(&out.Coordination)
}

func (in *CoactorMember) DeepCopyInto(out *CoactorMember) {
	*out = *in
	if in.DependsOn != nil {
		in, out := &in.DependsOn, &out.DependsOn
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *CoordinationConfig) DeepCopyInto(out *CoordinationConfig) {
	*out = *in
	if in.KVTransfer != nil {
		in, out := &in.KVTransfer, &out.KVTransfer
		*out = new(KVTransferConfig)
		**out = **in
	}
	if in.Aggregation != nil {
		in, out := &in.Aggregation, &out.Aggregation
		*out = new(AggregationConfig)
		**out = **in
	}
}

func (in *CoactorGroupStatus) DeepCopyInto(out *CoactorGroupStatus) {
	*out = *in
	if in.MemberStatuses != nil {
		in, out := &in.MemberStatuses, &out.MemberStatuses
		*out = make([]MemberStatus, len(*in))
		copy(*out, *in)
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func init() {
	SchemeBuilder.Register(&CoactorGroup{}, &CoactorGroupList{})
}
