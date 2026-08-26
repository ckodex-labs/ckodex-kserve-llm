/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package validation contains policy shared by admission and reconciliation.
package validation

import "fmt"

const (
	EngineVLLM     = "vllm"
	EngineQuantCpp = "quant-cpp"
)

// ValidateInferenceEngine rejects engines without an implemented runtime path.
func ValidateInferenceEngine(engine string) error {
	switch engine {
	case "", EngineVLLM, EngineQuantCpp:
		return nil
	default:
		return fmt.Errorf("unsupported inference engine %q: supported engines are %q and %q", engine, EngineVLLM, EngineQuantCpp)
	}
}
