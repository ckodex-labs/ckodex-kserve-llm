/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.modelRef`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.boundEndpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InferenceSession is the stable v1 API for stateful conversation contexts.
// It tracks KV-cache affinity to route subsequent turns to the same vLLM pod.
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

	// MaxTurns limits the number of conversation turns per session. 0 = unlimited.
	// +kubebuilder:default=0
	// +optional
	MaxTurns int32 `json:"maxTurns,omitempty"`

	// ActorRef optionally binds this session to a specific InferenceActor.
	// +optional
	ActorRef string `json:"actorRef,omitempty"`

	// CoactorGroupRef binds this session to a CoactorGroup for multi-model collaboration.
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
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// InferenceSessionList contains a list of InferenceSession.
type InferenceSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceSession `json:"items"`
}

// Hub marks InferenceSession as the conversion hub (storage version).
func (*InferenceSession) Hub() {}

// --- Actor types ---

// ActorType classifies actors.
type ActorType string

const (
	ActorTypeModel ActorType = "model"
	ActorTypeAgent ActorType = "agent"
	ActorTypeTool  ActorType = "tool"
)

// ActorState represents the actor lifecycle.
type ActorState string

const (
	ActorStateInactive     ActorState = "Inactive"
	ActorStateActivating   ActorState = "Activating"
	ActorStateActive       ActorState = "Active"
	ActorStateDeactivating ActorState = "Deactivating"
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.actorType`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// InferenceActor is the stable v1 API for Dapr virtual actors
// that own model or agent state within the operator.
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

	// AgentRef binds to an Agent (for agent actors).
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

	// StateStore is the Dapr state store component name for actor state.
	// +kubebuilder:default="statestore"
	// +optional
	StateStore string `json:"stateStore,omitempty"`
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
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// InferenceActorList contains a list of InferenceActor.
type InferenceActorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceActor `json:"items"`
}

// Hub marks InferenceActor as the conversion hub (storage version).
func (*InferenceActor) Hub() {}

func init() {
	SchemeBuilder.Register(
		&InferenceSession{}, &InferenceSessionList{},
		&InferenceActor{}, &InferenceActorList{},
	)
}
