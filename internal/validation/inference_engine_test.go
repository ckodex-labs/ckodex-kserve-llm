/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package validation

import "testing"

func TestValidateInferenceEngine(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		wantErr bool
	}{
		{name: "default", engine: ""},
		{name: "vllm", engine: EngineVLLM},
		{name: "quant cpp", engine: EngineQuantCpp},
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
