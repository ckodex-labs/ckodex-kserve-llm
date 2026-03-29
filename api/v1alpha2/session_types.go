/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.modelRef`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.boundEndpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InferenceSession tracks a stateful conversation context with KV-cache
// affinity. Routes subsequent turns to the same vLLM pod for prefix-cache reuse.
type InferenceSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InferenceSessionSpec   `json:"spec,omitempty"`
	Status            InferenceSessionStatus `json:"status,omitempty"`
}

// InferenceSessionSpec defines the desired session state.
type InferenceSessionSpec struct {
	// ModelRef references the LLMInferenceService serving this session.
	ModelRef string `json:"modelRef"`

	// TTL is the session time-to-live after last activity.
	// +kubebuilder:default="30m"
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// MaxTurns limits the number of conversation turns per session.
	// 0 = unlimited.
	// +kubebuilder:default=0
	// +optional
	MaxTurns int32 `json:"maxTurns,omitempty"`

	// ActorRef optionally binds this session to a specific actor.
	// +optional
	ActorRef string `json:"actorRef,omitempty"`

	// CoactorGroupRef binds this session to a coactor group for
	// multi-model collaboration.
	// +optional
	CoactorGroupRef string `json:"coactorGroupRef,omitempty"`

	// Metadata holds caller-supplied key-value pairs (user ID, conversation ID, etc).
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// InferenceSessionStatus defines the observed session state.
type InferenceSessionStatus struct {
	// Phase is the session lifecycle phase.
	Phase SessionPhase `json:"phase,omitempty"`

	// BoundEndpoint is the pod IP:port holding the KV-cache for this session.
	BoundEndpoint string `json:"boundEndpoint,omitempty"`

	// TurnCount tracks the number of completed turns.
	TurnCount int32 `json:"turnCount,omitempty"`

	// LastActivityTime is the timestamp of the last inference request.
	LastActivityTime *metav1.Time `json:"lastActivityTime,omitempty"`

	// KVCacheSize is the estimated KV-cache size in bytes.
	KVCacheSize int64 `json:"kvCacheSize,omitempty"`

	// TokenCount is the total tokens processed in this session.
	TokenCount int64 `json:"tokenCount,omitempty"`

	// Conditions track session health.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SessionPhase represents the session lifecycle.
type SessionPhase string

const (
	SessionPhaseActive    SessionPhase = "Active"
	SessionPhaseIdle      SessionPhase = "Idle"
	SessionPhaseDraining  SessionPhase = "Draining"
	SessionPhaseEvicted   SessionPhase = "Evicted"
	SessionPhaseCompleted SessionPhase = "Completed"
)

// +kubebuilder:object:root=true
type InferenceSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceSession `json:"items"`
}

// --- Actors ---

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.actorType`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// InferenceActor represents a Dapr virtual actor that owns model or agent state.
// Each LLMInferenceService or AgentSpec instance maps to one actor.
type InferenceActor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InferenceActorSpec   `json:"spec,omitempty"`
	Status            InferenceActorStatus `json:"status,omitempty"`
}

// InferenceActorSpec defines the actor configuration.
type InferenceActorSpec struct {
	// ActorType classifies this actor.
	ActorType ActorType `json:"actorType"`

	// ModelRef binds to an LLMInferenceService (for model actors).
	// +optional
	ModelRef string `json:"modelRef,omitempty"`

	// AgentRef binds to an AgentSpec (for agent actors).
	// +optional
	AgentRef string `json:"agentRef,omitempty"`

	// IdleTimeout deactivates the actor after inactivity.
	// +kubebuilder:default="10m"
	// +optional
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	// MaxConcurrency limits concurrent requests per actor instance.
	// +kubebuilder:default=1
	// +optional
	MaxConcurrency int32 `json:"maxConcurrency,omitempty"`

	// Reentrancy allows the actor to process multiple requests concurrently.
	// +kubebuilder:default=false
	// +optional
	Reentrancy bool `json:"reentrancy,omitempty"`

	// Reminders defines recurring timer-based callbacks.
	// +optional
	Reminders []ActorReminder `json:"reminders,omitempty"`

	// StateStore is the Dapr state store component for actor state.
	// +kubebuilder:default="statestore"
	// +optional
	StateStore string `json:"stateStore,omitempty"`
}

// ActorType classifies actors.
type ActorType string

const (
	ActorTypeModel ActorType = "model"
	ActorTypeAgent ActorType = "agent"
	ActorTypeTool  ActorType = "tool"
)

// ActorReminder defines a timer-based callback for an actor.
type ActorReminder struct {
	// Name of the reminder.
	Name string `json:"name"`
	// DueTime is when the reminder first fires.
	DueTime metav1.Duration `json:"dueTime"`
	// Period is the recurrence interval (empty = one-shot).
	// +optional
	Period *metav1.Duration `json:"period,omitempty"`
	// Data is opaque payload passed to the reminder callback.
	// +optional
	Data string `json:"data,omitempty"`
}

// InferenceActorStatus defines the observed actor state.
type InferenceActorStatus struct {
	// State is the actor lifecycle state.
	State ActorState `json:"state,omitempty"`

	// ActiveSessions tracks how many sessions are using this actor.
	ActiveSessions int32 `json:"activeSessions,omitempty"`

	// LastActivationTime is when the actor was last activated.
	LastActivationTime *metav1.Time `json:"lastActivationTime,omitempty"`

	// Conditions track actor health.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ActorState represents the actor lifecycle.
type ActorState string

const (
	ActorStateInactive     ActorState = "Inactive"
	ActorStateActivating   ActorState = "Activating"
	ActorStateActive       ActorState = "Active"
	ActorStateDeactivating ActorState = "Deactivating"
)

// +kubebuilder:object:root=true
type InferenceActorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceActor `json:"items"`
}

// --- Coactors ---

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pattern",type=string,JSONPath=`.spec.pattern`
// +kubebuilder:printcolumn:name="Members",type=integer,JSONPath=`.status.activeMemberCount`

// CoactorGroup defines a collaborative group of actors that work together
// on shared tasks — prefill+decode pairs, agent ensembles, or model pipelines.
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

// CoactorPattern defines the collaboration pattern.
type CoactorPattern string

const (
	// PatternPrefillDecode is disaggregated prefill + decode workers.
	PatternPrefillDecode CoactorPattern = "prefill-decode"
	// PatternAgentEnsemble is multiple agents collaborating on a task.
	PatternAgentEnsemble CoactorPattern = "agent-ensemble"
	// PatternPipeline is a sequential model pipeline (e.g., embed → retrieve → generate).
	PatternPipeline CoactorPattern = "pipeline"
	// PatternMixtureOfAgents is parallel agents with aggregation.
	PatternMixtureOfAgents CoactorPattern = "mixture-of-agents"
)

// CoactorMember defines a single member of the group.
type CoactorMember struct {
	// Name identifies this member within the group.
	Name string `json:"name"`

	// Role defines the member's role (e.g., "prefill", "decode", "router", "worker").
	Role string `json:"role"`

	// ActorRef references the InferenceActor.
	ActorRef string `json:"actorRef"`

	// Weight for load balancing within the group (for ensemble/mixture patterns).
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

	// KVTransfer configures KV-cache transfer between prefill/decode (for prefill-decode pattern).
	// +optional
	KVTransfer *KVTransferConfig `json:"kvTransfer,omitempty"`

	// Aggregation configures how results from ensemble members are combined.
	// +optional
	Aggregation *AggregationConfig `json:"aggregation,omitempty"`
}

// CoordinationMethod defines synchronization approach.
type CoordinationMethod string

const (
	CoordinationDirect   CoordinationMethod = "direct"   // Direct gRPC between actors
	CoordinationPubSub   CoordinationMethod = "pubsub"   // Dapr pub/sub
	CoordinationWorkflow CoordinationMethod = "workflow" // Dapr workflow orchestration
)

// KVTransferConfig defines KV-cache transfer settings.
type KVTransferConfig struct {
	// Method is the transfer mechanism: nccl, rdma, or tcp.
	// +kubebuilder:default="nccl"
	Method string `json:"method"`
}

// AggregationConfig defines how ensemble results are combined.
type AggregationConfig struct {
	// Strategy is the aggregation strategy.
	// +kubebuilder:default="best-of-n"
	Strategy string `json:"strategy"` // best-of-n, voting, weighted-merge, chain
}

// CoactorGroupStatus defines the observed state.
type CoactorGroupStatus struct {
	// Phase is the group lifecycle phase.
	Phase CoactorGroupPhase `json:"phase,omitempty"`

	// ActiveMemberCount is the number of active members.
	ActiveMemberCount int32 `json:"activeMemberCount,omitempty"`

	// MemberStatuses tracks each member's state.
	MemberStatuses []MemberStatus `json:"memberStatuses,omitempty"`

	// Conditions track group health.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CoactorGroupPhase represents the group lifecycle.
type CoactorGroupPhase string

const (
	CoactorGroupForming   CoactorGroupPhase = "Forming"
	CoactorGroupReady     CoactorGroupPhase = "Ready"
	CoactorGroupDegraded  CoactorGroupPhase = "Degraded"
	CoactorGroupDissolved CoactorGroupPhase = "Dissolved"
)

// MemberStatus tracks an individual member's state.
type MemberStatus struct {
	Name  string     `json:"name"`
	Role  string     `json:"role"`
	State ActorState `json:"state"`
	Ready bool       `json:"ready"`
}

// +kubebuilder:object:root=true
type CoactorGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CoactorGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&InferenceSession{}, &InferenceSessionList{},
		&InferenceActor{}, &InferenceActorList{},
		&CoactorGroup{}, &CoactorGroupList{},
	)
}
