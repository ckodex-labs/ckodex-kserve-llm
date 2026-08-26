/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package validation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
)

type crdSpecInventoryEntry struct {
	version string
	kind    string
	typeOf  reflect.Type
	fields  []string
}

func TestCRDSpecInventoryMatchesReflectedFields(t *testing.T) {
	for _, entry := range crdSpecInventory() {
		got := jsonFieldNames(entry.typeOf)
		want := append([]string(nil), entry.fields...)
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s %s fields differ: got %v, want %v", entry.version, entry.kind, got, want)
		}
	}
}

func TestCRDSpecInventoryMatchesRegisteredRootObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("register v1alpha2 types: %v", err)
	}
	if err := servingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("register v1 types: %v", err)
	}

	registered := map[string]bool{}
	for _, entry := range crdSpecInventory() {
		key := inventoryKey(entry.version, entry.kind)
		if registered[key] {
			t.Errorf("duplicate CRD surface inventory entry %s", key)
		}
		registered[key] = true
	}
	discovered := map[string]bool{}
	for gvk, typeOf := range scheme.AllKnownTypes() {
		if _, ok := typeOf.FieldByName("Spec"); !ok {
			continue
		}
		key := gvk.GroupVersion().String() + "#" + gvk.Kind
		discovered[key] = true
		if !registered[key] {
			t.Errorf("registered root %s is absent from CRD surface inventory", key)
		}
	}
	for key := range registered {
		if !discovered[key] {
			t.Errorf("CRD surface inventory entry %s is not registered in the API scheme", key)
		}
	}
}

func jsonFieldNames(typeOf reflect.Type) []string {
	fields := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	return fields
}

func inventoryKey(version, kind string) string {
	return fmt.Sprintf("serving.ckodex.com/%s#%s", version, kind)
}

func crdSpecInventory() []crdSpecInventoryEntry {
	return []crdSpecInventoryEntry{
		{version: "v1alpha2", kind: "Agent", typeOf: reflect.TypeOf(servingv1alpha2.AgentConfiguration{}), fields: []string{"identity", "modelRef", "skills", "tools", "maxTokens", "template"}},
		{version: "v1alpha2", kind: "SkillRegistry", typeOf: reflect.TypeOf(servingv1alpha2.SkillRegistrySpec{}), fields: []string{"entries"}},
		{version: "v1alpha2", kind: "ModelOnboarding", typeOf: reflect.TypeOf(servingv1alpha2.ModelOnboardingSpec{}), fields: []string{"modelRef", "stages", "rollbackOnFailure"}},
		{version: "v1alpha2", kind: "AIPack", typeOf: reflect.TypeOf(servingv1alpha2.AIPackSpec{}), fields: []string{"kind", "family", "source", "composition", "attestation", "policy", "baseModel", "lora", "fineTune", "skill", "tool", "mcpServer", "promptTemplate", "guardrail", "retrievalIndex", "dataset", "harness", "eval", "workflow", "policyBundle"}},
		{version: "v1alpha2", kind: "ASRInferenceService", typeOf: reflect.TypeOf(servingv1alpha2.ASRInferenceServiceSpec{}), fields: []string{"model", "runtime", "runtimeImage", "accelerator", "languages", "replicas", "scaling", "template"}},
		{version: "v1alpha2", kind: "EmbeddingInferenceService", typeOf: reflect.TypeOf(servingv1alpha2.EmbeddingInferenceServiceSpec{}), fields: []string{"model", "runtime", "runtimeImage", "accelerator", "replicas", "batchSize", "scaling", "template"}},
		{version: "v1alpha2", kind: "EndpointPickerConfig", typeOf: reflect.TypeOf(servingv1alpha2.EndpointPickerConfigSpec{}), fields: []string{"plugins", "schedulingProfiles"}},
		{version: "v1alpha2", kind: "EvalProfile", typeOf: reflect.TypeOf(servingv1alpha2.EvalProfileSpec{}), fields: []string{"promptSet", "mandatoryMetrics", "targetEngine", "maxDurationSeconds"}},
		{version: "v1alpha2", kind: "LLMInferenceService", typeOf: reflect.TypeOf(servingv1alpha2.LLMInferenceServiceSpec{}), fields: directSurfaceFields("v1alpha2")},
		{version: "v1alpha2", kind: "LLMInferenceServiceConfig", typeOf: reflect.TypeOf(servingv1alpha2.LLMInferenceServiceConfigSpec{}), fields: []string{"template", "router", "scaling", "parallelism", "worker", "vllmDefaults", "complianceProfiles"}},
		{version: "v1alpha2", kind: "LLMLoraAdapter", typeOf: reflect.TypeOf(servingv1alpha2.LLMLoraAdapterSpec{}), fields: []string{"targetService", "adapterName", "model", "behavior", "policyEnvelope", "toolSurface", "sandbox"}},
		{version: "v1alpha2", kind: "LocalModelCache", typeOf: reflect.TypeOf(servingv1alpha2.LocalModelCacheSpec{}), fields: []string{"sourceModelUri", "modelSize", "maxCacheSize", "nodeGroup", "warmNodes", "storage", "allowedNamespaces", "env", "storageClassName"}},
		{version: "v1alpha2", kind: "MultimodalInferenceService", typeOf: reflect.TypeOf(servingv1alpha2.MultimodalInferenceServiceSpec{}), fields: []string{"model", "task", "runtime", "runtimeImage", "maxImagesPerPrompt", "imageInputType", "imageProcessorModel", "quantization", "replicas", "scaling", "template"}},
		{version: "v1alpha2", kind: "RerankerInferenceService", typeOf: reflect.TypeOf(servingv1alpha2.RerankerInferenceServiceSpec{}), fields: []string{"model", "quantization", "maxCandidates", "resources", "replicas"}},
		{version: "v1alpha2", kind: "InferenceSession", typeOf: reflect.TypeOf(servingv1alpha2.InferenceSessionSpec{}), fields: []string{"modelRef", "ttl", "maxTurns", "actorRef", "coactorGroupRef", "metadata"}},
		{version: "v1alpha2", kind: "InferenceActor", typeOf: reflect.TypeOf(servingv1alpha2.InferenceActorSpec{}), fields: []string{"actorType", "modelRef", "agentRef", "idleTimeout", "maxConcurrency", "reentrancy", "reminders", "stateStore"}},
		{version: "v1alpha2", kind: "CoactorGroup", typeOf: reflect.TypeOf(servingv1alpha2.CoactorGroupSpec{}), fields: []string{"pattern", "members", "coordination", "sessionAffinity"}},
		{version: "v1", kind: "Agent", typeOf: reflect.TypeOf(servingv1.AgentConfiguration{}), fields: []string{"identity", "modelRef", "skills", "tools", "maxTokens", "template"}},
		{version: "v1", kind: "SkillRegistry", typeOf: reflect.TypeOf(servingv1.SkillRegistrySpec{}), fields: []string{"entries"}},
		{version: "v1", kind: "ModelOnboarding", typeOf: reflect.TypeOf(servingv1.ModelOnboardingSpec{}), fields: []string{"modelRef", "stages", "rollbackOnFailure"}},
		{version: "v1", kind: "LLMInferenceService", typeOf: reflect.TypeOf(servingv1.LLMInferenceServiceSpec{}), fields: directSurfaceFields("v1")},
		{version: "v1", kind: "LLMLoraAdapter", typeOf: reflect.TypeOf(servingv1.LLMLoraAdapterSpec{}), fields: []string{"targetService", "adapterName", "model"}},
		{version: "v1", kind: "InferenceSession", typeOf: reflect.TypeOf(servingv1.InferenceSessionSpec{}), fields: []string{"modelRef", "ttl", "maxTurns", "actorRef", "coactorGroupRef", "metadata"}},
		{version: "v1", kind: "InferenceActor", typeOf: reflect.TypeOf(servingv1.InferenceActorSpec{}), fields: []string{"actorType", "modelRef", "agentRef", "idleTimeout", "maxConcurrency", "reentrancy", "reminders", "stateStore"}},
		{version: "v1", kind: "CoactorGroup", typeOf: reflect.TypeOf(servingv1.CoactorGroupSpec{}), fields: []string{"pattern", "members", "coordination", "sessionAffinity"}},
	}
}

func directSurfaceFields(version string) []string {
	fields := []string{}
	for _, contract := range LLMInferenceServiceSurfaceContracts() {
		if contract.Version == version && !strings.Contains(contract.Path, ".") {
			fields = append(fields, contract.Path)
		}
	}
	return fields
}
