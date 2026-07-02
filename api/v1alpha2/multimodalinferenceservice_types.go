/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MultimodalTask describes the type of multimodal workload.
//
// +kubebuilder:validation:Enum=vision-language;image-generation;text-to-speech
type MultimodalTask string

const (
	// MultimodalTaskVisionLanguage serves vision-language models (VLMs) that accept
	// image + text inputs and produce text outputs (e.g. LLaVA, Qwen2-VL, InternVL2).
	// Exposes /v1/chat/completions with image_url content part support.
	MultimodalTaskVisionLanguage MultimodalTask = "vision-language"

	// MultimodalTaskImageGeneration serves text-to-image generation models
	// (e.g. SDXL, Flux) via a custom image generation endpoint.
	// Exposes /v1/images/generations.
	MultimodalTaskImageGeneration MultimodalTask = "image-generation"

	// MultimodalTaskTextToSpeech serves text-to-speech generation models
	// (e.g. LiquidAI/LFM2.5-Audio-1.5B) via a custom audio generation endpoint.
	// Exposes /v1/audio/speech.
	MultimodalTaskTextToSpeech MultimodalTask = "text-to-speech"
)

// MultimodalRuntime selects the serving runtime for multimodal models.
//
// +kubebuilder:validation:Enum=vllm
type MultimodalRuntime string

const (
	// MultimodalRuntimeVLLM uses vLLM, which natively supports vision-language models
	// through its multimodal input pipeline. Supports LLaVA, Qwen2-VL, InternVL2,
	// Pixtral, and other HuggingFace VLMs.
	// Exposes /v1/chat/completions with image_url content part support.
	MultimodalRuntimeVLLM MultimodalRuntime = "vllm"
)

// DefaultMultimodalRuntimeImage returns the default container image for a given runtime.
func DefaultMultimodalRuntimeImage(r MultimodalRuntime) string {
	switch r {
	case MultimodalRuntimeVLLM:
		return "vllm/vllm-openai:v0.24.0"
	default:
		return ""
	}
}

// MultimodalServerPort is the port on which the VLM runtime exposes its HTTP API.
const MultimodalServerPort = 8000

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mmsvc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Task",type="string",JSONPath=".spec.task"
// +kubebuilder:printcolumn:name="Runtime",type="string",JSONPath=".spec.runtime"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MultimodalInferenceService manages the lifecycle of multimodal model workloads.
// It creates a Deployment + Service for the selected runtime and exposes the
// OpenAI-compatible API for the requested task.
//
// Supported tasks:
//   - vision-language: VLMs that accept image+text, emit text (/v1/chat/completions)
//   - image-generation: text-to-image models (/v1/images/generations)
//   - text-to-speech: text-to-speech models (/v1/audio/speech)
type MultimodalInferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultimodalInferenceServiceSpec   `json:"spec,omitempty"`
	Status MultimodalInferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MultimodalInferenceServiceList contains a list of MultimodalInferenceService.
type MultimodalInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultimodalInferenceService `json:"items"`
}

// MultimodalInferenceServiceSpec defines the desired state of a MultimodalInferenceService.
type MultimodalInferenceServiceSpec struct {
	// Model specifies the multimodal model to serve.
	// For vision-language models: hf://llava-hf/llava-v1.6-mistral-7b-hf,
	//   hf://Qwen/Qwen2-VL-7B-Instruct, hf://OpenGVLab/InternVL2-8B.
	// For image-generation models: hf://stabilityai/stable-diffusion-xl-base-1.0.
	// For text-to-speech models: hf://LiquidAI/LFM2.5-Audio-1.5B.
	Model ModelSpec `json:"model"`

	// Task describes the type of multimodal workload. Defaults to vision-language.
	// +kubebuilder:default=vision-language
	// +kubebuilder:validation:Enum=vision-language;image-generation;text-to-speech
	// +optional
	Task MultimodalTask `json:"task,omitempty"`

	// Runtime selects the serving runtime. Currently only vllm is supported.
	// +kubebuilder:default=vllm
	// +kubebuilder:validation:Enum=vllm
	// +optional
	Runtime MultimodalRuntime `json:"runtime,omitempty"`

	// RuntimeImage overrides the default container image for the selected runtime.
	// When omitted, the operator uses vllm/vllm-openai:v0.24.0.
	// +optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	// MaxImagesPerPrompt limits the number of images accepted in a single request.
	// Maps to vLLM's --limit-mm-per-prompt flag (image=N).
	// Defaults to 1. Increase for models that support interleaved multi-image prompts.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16
	// +optional
	MaxImagesPerPrompt *int32 `json:"maxImagesPerPrompt,omitempty"`

	// ImageInputType selects the pixel encoding passed to the vision encoder.
	// Corresponds to vLLM --image-input-type (e.g., "pixel_values", "image_features").
	// Leave empty to let the model determine the encoding from its config.
	// +optional
	ImageInputType string `json:"imageInputType,omitempty"`

	// ImageProcessorModel overrides the image processor used for pixel encoding.
	// Defaults to the processor bundled with the model weights.
	// Useful when the vision encoder is separate from the language model.
	// +optional
	ImageProcessorModel string `json:"imageProcessorModel,omitempty"`

	// Quantization configures weight quantization for VLMs with large vision encoders.
	// +optional
	Quantization *QuantizationSpec `json:"quantization,omitempty"`

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
	// position 0. VLMs typically require GPU resources — set spec.template.spec.resources
	// accordingly.
	// +optional
	Template corev1.PodTemplateSpec `json:"template,omitempty"`
}

// MultimodalInferenceServiceStatus defines the observed state of a MultimodalInferenceService.
type MultimodalInferenceServiceStatus struct {
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// URL is the inference endpoint for the selected task.
	// vision-language:   http://<name>.<ns>.svc.cluster.local/v1/chat/completions
	// image-generation:  http://<name>.<ns>.svc.cluster.local/v1/images/generations
	// text-to-speech:    http://<name>.<ns>.svc.cluster.local/v1/audio/speech
	// +optional
	URL string `json:"url,omitempty"`

	// Replicas is the number of currently ready pods.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for MultimodalInferenceService.
const (
	// MultimodalConditionReady indicates the service is ready to serve requests.
	MultimodalConditionReady = "Ready"

	// MultimodalConditionDeploymentReady indicates the underlying Deployment is available.
	MultimodalConditionDeploymentReady = "DeploymentReady"
)

func init() {
	SchemeBuilder.Register(&MultimodalInferenceService{}, &MultimodalInferenceServiceList{})
}
