/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1

// Hub marks LLMInferenceService as the conversion hub (storage version).
// All other versions (v1alpha2, future v1beta1) convert through v1.
//
// controller-runtime requires the hub version to implement the Hub() method
// from the conversion.Hub interface. The method body is intentionally empty —
// its sole purpose is to satisfy the interface and act as a type marker.
func (*LLMInferenceService) Hub() {}

// Hub marks LLMLoraAdapter as the conversion hub.
func (*LLMLoraAdapter) Hub() {}

// Note: Hub() methods for Agent, SkillRegistry, ModelOnboarding, InferenceSession,
// InferenceActor, and CoactorGroup are defined in their respective type files.
