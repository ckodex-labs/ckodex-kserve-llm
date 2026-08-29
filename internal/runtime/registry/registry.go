/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package registry owns the immutable set of admitted LLM runtime adapters.
package registry

import (
	"sort"

	inferenceruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime"
	sglangruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/sglang"
	vllmruntime "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/vllm"
)

const (
	DefaultEngine = "vllm"
	SGLangEngine  = "sglang"
)

var adapters = map[string]inferenceruntime.Adapter{
	DefaultEngine: vllmruntime.Adapter{},
	SGLangEngine:  sglangruntime.Adapter{},
}

// Resolve returns the adapter admitted for name. An empty name resolves to the
// API default; unknown and unverified engines are absent from the registry.
func Resolve(name string) (inferenceruntime.Adapter, bool) {
	if name == "" {
		name = DefaultEngine
	}
	adapter, ok := adapters[name]
	return adapter, ok
}

// Names returns the admitted engine names in deterministic order.
func Names() []string {
	names := make([]string, 0, len(adapters))
	for name := range adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
