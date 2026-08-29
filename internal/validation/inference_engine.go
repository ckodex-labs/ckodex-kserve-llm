/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package validation contains policy shared by admission and reconciliation.
package validation

import (
	"fmt"
	"strings"

	runtimeregistry "github.com/ckodex-labs/kserve-llm-operator/internal/runtime/registry"
)

const (
	EngineVLLM   = runtimeregistry.DefaultEngine
	EngineSGLang = runtimeregistry.SGLangEngine
)

// ValidateInferenceEngine rejects engines without an implemented runtime path.
func ValidateInferenceEngine(engine string) error {
	if _, ok := runtimeregistry.Resolve(engine); ok {
		return nil
	}
	return fmt.Errorf("unsupported inference engine %q: admitted engines are %s", engine, quotedEngineNames())
}

// AdmittedInferenceEngines returns the engine values accepted by admission.
func AdmittedInferenceEngines() []string {
	return runtimeregistry.Names()
}

func quotedEngineNames() string {
	names := AdmittedInferenceEngines()
	for index := range names {
		names[index] = fmt.Sprintf("%q", names[index])
	}
	return strings.Join(names, ", ")
}
