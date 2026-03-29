/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// conversion_extra.go implements hub-and-spoke conversions for:
//   Agent, SkillRegistry, ModelOnboarding,
//   InferenceSession, InferenceActor, CoactorGroup.
//
// All field sets are structurally identical between v1alpha2 and v1
// (v1 was defined from v1alpha2). Future divergences must be added here.

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ─── Agent ──────────────────────────────────────────────────────────────────

func (src *Agent) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.Agent)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = servingv1.AgentConfiguration{
		Identity: servingv1.AgentIdentity{
			Name:        src.Spec.Identity.Name,
			Description: src.Spec.Identity.Description,
			Version:     src.Spec.Identity.Version,
		},
		ModelRef:  src.Spec.ModelRef,
		MaxTokens: src.Spec.MaxTokens,
		Template:  src.Spec.Template,
	}
	for _, s := range src.Spec.Skills {
		dst.Spec.Skills = append(dst.Spec.Skills, servingv1.SkillRef{
			RegistryRef: s.RegistryRef,
			SkillName:   s.SkillName,
			Version:     s.Version,
		})
	}
	for _, t := range src.Spec.Tools {
		dst.Spec.Tools = append(dst.Spec.Tools, servingv1.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Endpoint:    t.Endpoint,
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.Ready = src.Status.Ready
	return nil
}

func (dst *Agent) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.Agent)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = AgentConfiguration{
		Identity: AgentIdentity{
			Name:        src.Spec.Identity.Name,
			Description: src.Spec.Identity.Description,
			Version:     src.Spec.Identity.Version,
		},
		ModelRef:  src.Spec.ModelRef,
		MaxTokens: src.Spec.MaxTokens,
		Template:  src.Spec.Template,
	}
	for _, s := range src.Spec.Skills {
		dst.Spec.Skills = append(dst.Spec.Skills, SkillRef{
			RegistryRef: s.RegistryRef,
			SkillName:   s.SkillName,
			Version:     s.Version,
		})
	}
	for _, t := range src.Spec.Tools {
		dst.Spec.Tools = append(dst.Spec.Tools, ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Endpoint:    t.Endpoint,
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.Ready = src.Status.Ready
	return nil
}

// ─── SkillRegistry ──────────────────────────────────────────────────────────

func (src *SkillRegistry) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.SkillRegistry)
	dst.ObjectMeta = src.ObjectMeta
	for _, e := range src.Spec.Entries {
		dst.Spec.Entries = append(dst.Spec.Entries, servingv1.SkillEntry{
			Name:         e.Name,
			Version:      e.Version,
			Description:  e.Description,
			Endpoint:     e.Endpoint,
			InputSchema:  e.InputSchema,
			Capabilities: append([]string(nil), e.Capabilities...),
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.EntryCount = src.Status.EntryCount
	return nil
}

func (dst *SkillRegistry) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.SkillRegistry)
	dst.ObjectMeta = src.ObjectMeta
	for _, e := range src.Spec.Entries {
		dst.Spec.Entries = append(dst.Spec.Entries, SkillEntry{
			Name:         e.Name,
			Version:      e.Version,
			Description:  e.Description,
			Endpoint:     e.Endpoint,
			InputSchema:  e.InputSchema,
			Capabilities: append([]string(nil), e.Capabilities...),
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.EntryCount = src.Status.EntryCount
	return nil
}

// ─── ModelOnboarding ────────────────────────────────────────────────────────

func (src *ModelOnboarding) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.ModelOnboarding)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ModelRef = src.Spec.ModelRef
	dst.Spec.RollbackOnFailure = src.Spec.RollbackOnFailure
	for _, s := range src.Spec.Stages {
		stage := servingv1.OnboardingStage{Name: s.Name, Type: s.Type}
		if s.Gate != nil {
			stage.Gate = &servingv1.GateCriteria{
				MinSuccessRate: s.Gate.MinSuccessRate,
			}
			if s.Gate.MaxLatencyP99 != nil {
				v := *s.Gate.MaxLatencyP99
				stage.Gate.MaxLatencyP99 = &v
			}
		}
		dst.Spec.Stages = append(dst.Spec.Stages, stage)
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.CurrentStage = src.Status.CurrentStage
	dst.Status.Phase = src.Status.Phase
	return nil
}

func (dst *ModelOnboarding) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.ModelOnboarding)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ModelRef = src.Spec.ModelRef
	dst.Spec.RollbackOnFailure = src.Spec.RollbackOnFailure
	for _, s := range src.Spec.Stages {
		stage := OnboardingStage{Name: s.Name, Type: s.Type}
		if s.Gate != nil {
			stage.Gate = &GateCriteria{
				MinSuccessRate: s.Gate.MinSuccessRate,
			}
			if s.Gate.MaxLatencyP99 != nil {
				v := *s.Gate.MaxLatencyP99
				stage.Gate.MaxLatencyP99 = &v
			}
		}
		dst.Spec.Stages = append(dst.Spec.Stages, stage)
	}
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.CurrentStage = src.Status.CurrentStage
	dst.Status.Phase = src.Status.Phase
	return nil
}

// ─── InferenceSession ───────────────────────────────────────────────────────

func (src *InferenceSession) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.InferenceSession)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ModelRef = src.Spec.ModelRef
	dst.Spec.TTL = src.Spec.TTL
	dst.Spec.MaxTurns = src.Spec.MaxTurns
	dst.Spec.ActorRef = src.Spec.ActorRef
	dst.Spec.CoactorGroupRef = src.Spec.CoactorGroupRef
	if src.Spec.Metadata != nil {
		dst.Spec.Metadata = make(map[string]string, len(src.Spec.Metadata))
		for k, v := range src.Spec.Metadata {
			dst.Spec.Metadata[k] = v
		}
	}
	dst.Status.Phase = servingv1.SessionPhase(src.Status.Phase)
	dst.Status.BoundEndpoint = src.Status.BoundEndpoint
	dst.Status.TurnCount = src.Status.TurnCount
	dst.Status.LastActivityTime = src.Status.LastActivityTime
	dst.Status.KVCacheSize = src.Status.KVCacheSize
	dst.Status.TokenCount = src.Status.TokenCount
	dst.Status.Conditions = src.Status.Conditions
	return nil
}

func (dst *InferenceSession) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.InferenceSession)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ModelRef = src.Spec.ModelRef
	dst.Spec.TTL = src.Spec.TTL
	dst.Spec.MaxTurns = src.Spec.MaxTurns
	dst.Spec.ActorRef = src.Spec.ActorRef
	dst.Spec.CoactorGroupRef = src.Spec.CoactorGroupRef
	if src.Spec.Metadata != nil {
		dst.Spec.Metadata = make(map[string]string, len(src.Spec.Metadata))
		for k, v := range src.Spec.Metadata {
			dst.Spec.Metadata[k] = v
		}
	}
	dst.Status.Phase = SessionPhase(src.Status.Phase)
	dst.Status.BoundEndpoint = src.Status.BoundEndpoint
	dst.Status.TurnCount = src.Status.TurnCount
	dst.Status.LastActivityTime = src.Status.LastActivityTime
	dst.Status.KVCacheSize = src.Status.KVCacheSize
	dst.Status.TokenCount = src.Status.TokenCount
	dst.Status.Conditions = src.Status.Conditions
	return nil
}

// ─── InferenceActor ─────────────────────────────────────────────────────────

func (src *InferenceActor) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.InferenceActor)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = servingv1.InferenceActorSpec{
		ActorType:      servingv1.ActorType(src.Spec.ActorType),
		ModelRef:       src.Spec.ModelRef,
		AgentRef:       src.Spec.AgentRef,
		IdleTimeout:    src.Spec.IdleTimeout,
		MaxConcurrency: src.Spec.MaxConcurrency,
		Reentrancy:     src.Spec.Reentrancy,
		StateStore:     src.Spec.StateStore,
	}
	for _, r := range src.Spec.Reminders {
		rem := servingv1.ActorReminder{
			Name:    r.Name,
			DueTime: r.DueTime,
			Data:    r.Data,
		}
		if r.Period != nil {
			p := *r.Period
			rem.Period = &p
		}
		dst.Spec.Reminders = append(dst.Spec.Reminders, rem)
	}
	dst.Status.State = servingv1.ActorState(src.Status.State)
	dst.Status.ActiveSessions = src.Status.ActiveSessions
	dst.Status.LastActivationTime = src.Status.LastActivationTime
	dst.Status.Conditions = src.Status.Conditions
	return nil
}

func (dst *InferenceActor) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.InferenceActor)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = InferenceActorSpec{
		ActorType:      ActorType(src.Spec.ActorType),
		ModelRef:       src.Spec.ModelRef,
		AgentRef:       src.Spec.AgentRef,
		IdleTimeout:    src.Spec.IdleTimeout,
		MaxConcurrency: src.Spec.MaxConcurrency,
		Reentrancy:     src.Spec.Reentrancy,
		StateStore:     src.Spec.StateStore,
	}
	for _, r := range src.Spec.Reminders {
		rem := ActorReminder{
			Name:    r.Name,
			DueTime: r.DueTime,
			Data:    r.Data,
		}
		if r.Period != nil {
			p := *r.Period
			rem.Period = &p
		}
		dst.Spec.Reminders = append(dst.Spec.Reminders, rem)
	}
	dst.Status.State = ActorState(src.Status.State)
	dst.Status.ActiveSessions = src.Status.ActiveSessions
	dst.Status.LastActivationTime = src.Status.LastActivationTime
	dst.Status.Conditions = src.Status.Conditions
	return nil
}

// ─── CoactorGroup ───────────────────────────────────────────────────────────

func (src *CoactorGroup) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.CoactorGroup)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Pattern = servingv1.CoactorPattern(src.Spec.Pattern)
	dst.Spec.SessionAffinity = src.Spec.SessionAffinity
	for _, m := range src.Spec.Members {
		member := servingv1.CoactorMember{
			Name:      m.Name,
			Role:      m.Role,
			ActorRef:  m.ActorRef,
			Weight:    m.Weight,
			DependsOn: append([]string(nil), m.DependsOn...),
		}
		dst.Spec.Members = append(dst.Spec.Members, member)
	}
	dst.Spec.Coordination = servingv1.CoordinationConfig{
		Method: servingv1.CoordinationMethod(src.Spec.Coordination.Method),
	}
	if src.Spec.Coordination.KVTransfer != nil {
		dst.Spec.Coordination.KVTransfer = &servingv1.KVTransferConfig{
			Method: src.Spec.Coordination.KVTransfer.Method,
		}
	}
	if src.Spec.Coordination.Aggregation != nil {
		dst.Spec.Coordination.Aggregation = &servingv1.AggregationConfig{
			Strategy: src.Spec.Coordination.Aggregation.Strategy,
		}
	}
	dst.Status.Phase = servingv1.CoactorGroupPhase(src.Status.Phase)
	dst.Status.ActiveMemberCount = src.Status.ActiveMemberCount
	for _, ms := range src.Status.MemberStatuses {
		dst.Status.MemberStatuses = append(dst.Status.MemberStatuses, servingv1.MemberStatus{
			Name: ms.Name, Role: ms.Role,
			State: servingv1.ActorState(ms.State), Ready: ms.Ready,
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	return nil
}

func (dst *CoactorGroup) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.CoactorGroup)
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Pattern = CoactorPattern(src.Spec.Pattern)
	dst.Spec.SessionAffinity = src.Spec.SessionAffinity
	for _, m := range src.Spec.Members {
		member := CoactorMember{
			Name:      m.Name,
			Role:      m.Role,
			ActorRef:  m.ActorRef,
			Weight:    m.Weight,
			DependsOn: append([]string(nil), m.DependsOn...),
		}
		dst.Spec.Members = append(dst.Spec.Members, member)
	}
	dst.Spec.Coordination = CoordinationConfig{
		Method: CoordinationMethod(src.Spec.Coordination.Method),
	}
	if src.Spec.Coordination.KVTransfer != nil {
		dst.Spec.Coordination.KVTransfer = &KVTransferConfig{
			Method: src.Spec.Coordination.KVTransfer.Method,
		}
	}
	if src.Spec.Coordination.Aggregation != nil {
		dst.Spec.Coordination.Aggregation = &AggregationConfig{
			Strategy: src.Spec.Coordination.Aggregation.Strategy,
		}
	}
	dst.Status.Phase = CoactorGroupPhase(src.Status.Phase)
	dst.Status.ActiveMemberCount = src.Status.ActiveMemberCount
	for _, ms := range src.Status.MemberStatuses {
		dst.Status.MemberStatuses = append(dst.Status.MemberStatuses, MemberStatus{
			Name: ms.Name, Role: ms.Role,
			State: ActorState(ms.State), Ready: ms.Ready,
		})
	}
	dst.Status.Conditions = src.Status.Conditions
	return nil
}
