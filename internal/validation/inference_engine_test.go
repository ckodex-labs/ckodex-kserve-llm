/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package validation

import (
	"reflect"
	"testing"
)

func TestValidateInferenceEngine(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		wantErr bool
	}{
		{name: "default", engine: ""},
		{name: "vllm", engine: EngineVLLM},
		{name: "sglang", engine: EngineSGLang},
		{name: "unverified quant cpp", engine: "quant-cpp", wantErr: true},
		{name: "unsupported", engine: "other", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInferenceEngine(test.engine)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateInferenceEngine(%q) error = %v, wantErr %t", test.engine, err, test.wantErr)
			}
		})
	}
}

func TestAdmittedInferenceEnginesReturnsDefensiveCopy(t *testing.T) {
	engines := AdmittedInferenceEngines()
	want := []string{EngineSGLang, EngineVLLM}
	if !reflect.DeepEqual(engines, want) {
		t.Fatalf("AdmittedInferenceEngines() = %v, want %v", engines, want)
	}
	engines[0] = "mutated"
	if got := AdmittedInferenceEngines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("AdmittedInferenceEngines() leaked mutable state: %v", got)
	}
}
