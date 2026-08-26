/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ASRRuntime selects the serving runtime for ASR models.
// Different runtimes support different model architectures.
//
// +kubebuilder:validation:Enum=faster-whisper;transformers;custom
type ASRRuntime string

const (
	// ASRRuntimeFasterWhisper uses Speaches, the maintained successor to
	// faster-whisper-server. It requires CTranslate2-compatible Whisper models,
	// such as Systran/faster-whisper-*.
	// Exposes the OpenAI-compatible /v1/audio/transcriptions endpoint on port 8000.
	ASRRuntimeFasterWhisper ASRRuntime = "faster-whisper"

	// ASRRuntimeTransformers uses a HuggingFace Transformers-based inference server.
	// Required for custom ASR architectures that are not Whisper-based, such as
	// CohereLabs/cohere-transcribe-03-2026. The user must supply spec.runtimeImage
	// pointing to an image that exposes /v1/audio/transcriptions on port 8000.
	ASRRuntimeTransformers ASRRuntime = "transformers"

	// ASRRuntimeCustom accepts an operator-supplied image for runtimes such as
	// NVIDIA Parakeet, Canary, or other ASR servers that expose the documented
	// OpenAI-compatible transcription endpoint.
	ASRRuntimeCustom ASRRuntime = "custom"
)

// DefaultASRRuntimeImage returns the default container image for each runtime.
// An empty string means no default is available — the user must set spec.runtimeImage.
func DefaultASRRuntimeImage(r ASRRuntime) string {
	switch r {
	case ASRRuntimeFasterWhisper:
		// Speaches v0.9.0-rc.3 CPU multi-platform index (amd64 + arm64).
		return "ghcr.io/speaches-ai/speaches@sha256:2163775b6df5e451a71200e8f675fed68dbd8ab184fc604453d549e486f22fd2"
	default:
		// ASRRuntimeTransformers has no canonical public default image in this repo.
		// Users must set spec.runtimeImage explicitly for that runtime.
		return ""
	}
}

// DefaultASRAcceleratorImage returns a registry-verified CUDA image pinned by
// OCI digest. Transformers still requires an explicit runtimeImage.
func DefaultASRAcceleratorImage(r ASRRuntime) string {
	if r == ASRRuntimeFasterWhisper {
		return "ghcr.io/speaches-ai/speaches@sha256:6ec12ebf890a17e0d4b242a8ba9e0eb1fb836e60e8a3c857aea9838d541579ac"
	}
	return ""
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=asrsvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Runtime",type="string",JSONPath=".spec.runtime"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ASRInferenceService manages the lifecycle of automatic speech recognition
// workloads. It creates a Deployment + Service for the selected runtime and
// exposes the OpenAI-compatible /v1/audio/transcriptions endpoint.
//
// Two runtime modes are supported:
//   - faster-whisper: for Whisper-family models (default, no GPU required)
//   - transformers:   for custom ASR architectures (e.g. CohereLabs/cohere-transcribe-03-2026)
//   - custom:         for operator-supplied ASR images (e.g. NVIDIA Parakeet)
type ASRInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ASRInferenceServiceSpec   `json:"spec,omitempty"`
	Status ASRInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ASRInferenceServiceList contains a list of ASRInferenceService.
type ASRInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ASRInferenceService `json:"items"`
}

// ASRInferenceServiceSpec defines the desired state of an ASRInferenceService.
type ASRInferenceServiceSpec struct {
	// Model specifies the ASR model to serve.
	// For faster-whisper, use a CTranslate2 model such as
	// hf://Systran/faster-whisper-large-v3.
	// For transformers, use hf://CohereLabs/cohere-transcribe-03-2026.
	Model ModelSpec `json:"model"`

	// Runtime selects the serving runtime. Defaults to faster-whisper.
	// +kubebuilder:default=faster-whisper
	// +kubebuilder:validation:Enum=faster-whisper;transformers;custom
	// +optional
	Runtime ASRRuntime `json:"runtime,omitempty"`

	// RuntimeImage overrides the default container image for the selected runtime.
	// Required when runtime=transformers or custom (the operator cannot select a
	// safe image for an arbitrary ASR server).
	// When omitted for faster-whisper the operator uses the built-in default.
	// +optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	// Accelerator requests GPU resources and selects a digest-pinned CUDA image
	// when runtimeImage is omitted.
	// +optional
	Accelerator *AcceleratorSpec `json:"accelerator,omitempty"`

	// Languages lists the BCP-47 / ISO 639-1 language codes passed to custom
	// transformers runtimes. Speaches selects language per transcription request,
	// so this field is ignored when runtime is faster-whisper.
	// When empty, the runtime processes all languages the model supports.
	// Example: ["en", "fr", "de"]
	// +optional
	Languages []string `json:"languages,omitempty"`

	// Replicas is the desired number of serving pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Scaling configures autoscaling behaviour (HPA / KEDA).
	// +optional
	Scaling *ScalingSpec `json:"scaling,omitempty"`

	// Template allows customising the pod template (resources, tolerations,
	// node selectors, additional sidecars, etc.).
	// The operator enforces "Guaranteed QoS" by synchronizing resource requests
	// to match limits. If TerminationGracePeriodSeconds is not specified, it
	// defaults to 30s. The operator injects the primary runtime container at
	// position 0; containers defined here are appended as sidecars.
	// Omit the template to use the operator-managed pod defaults.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// ASRInferenceServiceStatus defines the observed state of an ASRInferenceService.
type ASRInferenceServiceStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// URL is the transcription endpoint.
	// Example: http://my-asr.ckodex-inference.svc.cluster.local/v1/audio/transcriptions
	// +optional
	URL string `json:"url,omitempty"`

	// Replicas is the number of currently ready pods.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for ASRInferenceService.
const (
	// ASRConditionReady indicates the service is ready to transcribe audio.
	ASRConditionReady = "Ready"

	// ASRConditionDeploymentReady indicates the underlying Deployment is available.
	ASRConditionDeploymentReady = "DeploymentReady"

	// ASRConditionModelLoaded indicates the model has been downloaded and loaded.
	ASRConditionModelLoaded = "ModelLoaded"
)

func init() {
	SchemeBuilder.Register(&ASRInferenceService{}, &ASRInferenceServiceList{})
}
