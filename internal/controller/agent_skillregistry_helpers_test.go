/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func newControllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	return s
}

func skillReg(name string, entries ...servingv1alpha2.SkillEntry) *servingv1alpha2.SkillRegistry {
	return &servingv1alpha2.SkillRegistry{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Spec: servingv1alpha2.SkillRegistrySpec{Entries: entries}}
}

func validEntry(name string) servingv1alpha2.SkillEntry {
	return servingv1alpha2.SkillEntry{Name: name, Version: "1.0.0", Endpoint: "http://" + name + ":8080", Description: name}
}

func readyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Status: servingv1alpha2.LLMInferenceServiceStatus{ModelReady: true}}
}

func notReadyLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	svc := readyLLMSvc(name)
	svc.Status.ModelReady = false
	return svc
}

func makeSkillReg(name string, skills ...string) *servingv1alpha2.SkillRegistry {
	entries := make([]servingv1alpha2.SkillEntry, 0, len(skills))
	for _, skill := range skills {
		entries = append(entries, validEntry(skill))
	}
	return skillReg(name, entries...)
}

func makeAgent(name, modelRef string, skills ...servingv1alpha2.SkillRef) *servingv1alpha2.Agent {
	return &servingv1alpha2.Agent{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Spec: servingv1alpha2.AgentConfiguration{Identity: servingv1alpha2.AgentIdentity{Name: name}, ModelRef: modelRef, Skills: skills}}
}
