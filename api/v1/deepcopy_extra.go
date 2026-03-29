/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// deepcopy_extra.go contains hand-written DeepCopy implementations for types
// added after the initial controller-gen run.
//
// TODO(ckodex): regenerate zz_generated.deepcopy.go via `make generate` once
// the dev toolchain is bootstrapped in CI. At that point this file can be deleted.
package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// --- Agent ---

func (in *Agent) DeepCopyInto(out *Agent) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *Agent) DeepCopy() *Agent {
	if in == nil {
		return nil
	}
	out := new(Agent)
	in.DeepCopyInto(out)
	return out
}

func (in *Agent) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AgentList) DeepCopyInto(out *AgentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Agent, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *AgentList) DeepCopy() *AgentList {
	if in == nil {
		return nil
	}
	out := new(AgentList)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AgentConfiguration) DeepCopyInto(out *AgentConfiguration) {
	*out = *in
	out.Identity = in.Identity
	if in.Skills != nil {
		in, out := &in.Skills, &out.Skills
		*out = make([]SkillRef, len(*in))
		copy(*out, *in)
	}
	if in.Tools != nil {
		in, out := &in.Tools, &out.Tools
		*out = make([]ToolDefinition, len(*in))
		copy(*out, *in)
	}
	if in.Template != nil {
		in, out := &in.Template, &out.Template
		*out = new(corev1.PodTemplateSpec)
		(*in).DeepCopyInto(*out)
	}
}

func (in *AgentStatus) DeepCopyInto(out *AgentStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// --- SkillRegistry ---

func (in *SkillRegistry) DeepCopyInto(out *SkillRegistry) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *SkillRegistry) DeepCopy() *SkillRegistry {
	if in == nil {
		return nil
	}
	out := new(SkillRegistry)
	in.DeepCopyInto(out)
	return out
}

func (in *SkillRegistry) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *SkillRegistryList) DeepCopyInto(out *SkillRegistryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]SkillRegistry, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *SkillRegistryList) DeepCopy() *SkillRegistryList {
	if in == nil {
		return nil
	}
	out := new(SkillRegistryList)
	in.DeepCopyInto(out)
	return out
}

func (in *SkillRegistryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *SkillRegistrySpec) DeepCopyInto(out *SkillRegistrySpec) {
	*out = *in
	if in.Entries != nil {
		in, out := &in.Entries, &out.Entries
		*out = make([]SkillEntry, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *SkillEntry) DeepCopyInto(out *SkillEntry) {
	*out = *in
	if in.Capabilities != nil {
		in, out := &in.Capabilities, &out.Capabilities
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *SkillRegistryStatus) DeepCopyInto(out *SkillRegistryStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// --- ModelOnboarding ---

func (in *ModelOnboarding) DeepCopyInto(out *ModelOnboarding) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ModelOnboarding) DeepCopy() *ModelOnboarding {
	if in == nil {
		return nil
	}
	out := new(ModelOnboarding)
	in.DeepCopyInto(out)
	return out
}

func (in *ModelOnboarding) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ModelOnboardingList) DeepCopyInto(out *ModelOnboardingList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ModelOnboarding, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ModelOnboardingList) DeepCopy() *ModelOnboardingList {
	if in == nil {
		return nil
	}
	out := new(ModelOnboardingList)
	in.DeepCopyInto(out)
	return out
}

func (in *ModelOnboardingList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ModelOnboardingSpec) DeepCopyInto(out *ModelOnboardingSpec) {
	*out = *in
	if in.Stages != nil {
		in, out := &in.Stages, &out.Stages
		*out = make([]OnboardingStage, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *OnboardingStage) DeepCopyInto(out *OnboardingStage) {
	*out = *in
	if in.Gate != nil {
		in, out := &in.Gate, &out.Gate
		*out = new(GateCriteria)
		(*in).DeepCopyInto(*out)
	}
}

func (in *GateCriteria) DeepCopyInto(out *GateCriteria) {
	*out = *in
	if in.MaxLatencyP99 != nil {
		in, out := &in.MaxLatencyP99, &out.MaxLatencyP99
		*out = new(int64)
		**out = **in
	}
}

func (in *ModelOnboardingStatus) DeepCopyInto(out *ModelOnboardingStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// --- InferenceSession ---

func (in *InferenceSession) DeepCopyInto(out *InferenceSession) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *InferenceSession) DeepCopy() *InferenceSession {
	if in == nil {
		return nil
	}
	out := new(InferenceSession)
	in.DeepCopyInto(out)
	return out
}

func (in *InferenceSession) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *InferenceSessionList) DeepCopyInto(out *InferenceSessionList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]InferenceSession, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *InferenceSessionList) DeepCopy() *InferenceSessionList {
	if in == nil {
		return nil
	}
	out := new(InferenceSessionList)
	in.DeepCopyInto(out)
	return out
}

func (in *InferenceSessionList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *InferenceSessionSpec) DeepCopyInto(out *InferenceSessionSpec) {
	*out = *in
	if in.TTL != nil {
		in, out := &in.TTL, &out.TTL
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.Metadata != nil {
		in, out := &in.Metadata, &out.Metadata
		*out = make(map[string]string, len(*in))
		for k, v := range *in {
			(*out)[k] = v
		}
	}
}

func (in *InferenceSessionStatus) DeepCopyInto(out *InferenceSessionStatus) {
	*out = *in
	if in.LastActivityTime != nil {
		in, out := &in.LastActivityTime, &out.LastActivityTime
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// --- InferenceActor ---

func (in *InferenceActor) DeepCopyInto(out *InferenceActor) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *InferenceActor) DeepCopy() *InferenceActor {
	if in == nil {
		return nil
	}
	out := new(InferenceActor)
	in.DeepCopyInto(out)
	return out
}

func (in *InferenceActor) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *InferenceActorList) DeepCopyInto(out *InferenceActorList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]InferenceActor, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *InferenceActorList) DeepCopy() *InferenceActorList {
	if in == nil {
		return nil
	}
	out := new(InferenceActorList)
	in.DeepCopyInto(out)
	return out
}

func (in *InferenceActorList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *InferenceActorSpec) DeepCopyInto(out *InferenceActorSpec) {
	*out = *in
	if in.IdleTimeout != nil {
		in, out := &in.IdleTimeout, &out.IdleTimeout
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.Reminders != nil {
		in, out := &in.Reminders, &out.Reminders
		*out = make([]ActorReminder, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ActorReminder) DeepCopyInto(out *ActorReminder) {
	*out = *in
	out.DueTime = in.DueTime
	if in.Period != nil {
		in, out := &in.Period, &out.Period
		*out = new(metav1.Duration)
		**out = **in
	}
}

func (in *InferenceActorStatus) DeepCopyInto(out *InferenceActorStatus) {
	*out = *in
	if in.LastActivationTime != nil {
		in, out := &in.LastActivationTime, &out.LastActivationTime
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}
