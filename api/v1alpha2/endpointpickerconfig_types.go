/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=epc

// EndpointPickerConfig defines the scheduler plugin pipeline for
// KV-cache aware request routing. The EPP uses this to score and
// select inference pods based on prefix cache hits and load.
type EndpointPickerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec EndpointPickerConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointPickerConfigList contains a list of EndpointPickerConfig.
type EndpointPickerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointPickerConfig `json:"items"`
}

// EndpointPickerConfigSpec defines the scheduler plugin pipeline.
type EndpointPickerConfigSpec struct {
	// Plugins defines the available scheduler plugins.
	Plugins []SchedulerPlugin `json:"plugins"`

	// SchedulingProfiles defines named profiles with weighted plugin references.
	SchedulingProfiles []SchedulingProfile `json:"schedulingProfiles"`
}

// SchedulerPlugin defines an available scheduler plugin.
type SchedulerPlugin struct {
	// Type identifies the plugin. Standard types:
	//   single-profile-handler, prefix-cache-scorer, precise-prefix-cache-scorer,
	//   load-aware-scorer, queue-scorer, kv-cache-utilization-scorer, max-score-picker.
	// Custom plugin types are also accepted for extensibility.
	Type string `json:"type"`

	// Parameters are optional plugin-specific parameters.
	// Supports nested structures for complex plugin configs
	// (e.g., kvEventsConfig, indexerConfig).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *PluginParameters `json:"parameters,omitempty"`
}

// PluginParameters holds arbitrary nested plugin configuration.
// +kubebuilder:pruning:PreserveUnknownFields
type PluginParameters struct {
	// Raw holds the raw JSON bytes of plugin parameters.
	Raw json.RawMessage `json:"-"`
}

// MarshalJSON implements json.Marshaler.
func (p PluginParameters) MarshalJSON() ([]byte, error) {
	if p.Raw == nil {
		return []byte("{}"), nil
	}
	return p.Raw, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *PluginParameters) UnmarshalJSON(data []byte) error {
	p.Raw = make(json.RawMessage, len(data))
	copy(p.Raw, data)
	return nil
}

// DeepCopyInto copies all properties into another PluginParameters.
func (p *PluginParameters) DeepCopyInto(out *PluginParameters) {
	if p.Raw != nil {
		out.Raw = make(json.RawMessage, len(p.Raw))
		copy(out.Raw, p.Raw)
	}
}

// DeepCopy returns a deep copy.
func (p *PluginParameters) DeepCopy() *PluginParameters {
	if p == nil {
		return nil
	}
	out := new(PluginParameters)
	p.DeepCopyInto(out)
	return out
}

// SchedulingProfile defines a named scheduling profile with weighted scorers.
type SchedulingProfile struct {
	// Name identifies this profile.
	Name string `json:"name"`

	// Plugins are the weighted plugin references for this profile.
	Plugins []WeightedPluginRef `json:"plugins"`
}

// WeightedPluginRef references a plugin with a scoring weight.
type WeightedPluginRef struct {
	// PluginRef is the plugin type to reference.
	PluginRef string `json:"pluginRef"`

	// Weight is the scoring weight for this plugin (as a string, e.g. "2.0").
	// Higher weight = more influence on final score.
	// +kubebuilder:default="1.0"
	// +optional
	Weight string `json:"weight,omitempty"`
}

func init() {
	SchemeBuilder.Register(&EndpointPickerConfig{}, &EndpointPickerConfigList{})
}
