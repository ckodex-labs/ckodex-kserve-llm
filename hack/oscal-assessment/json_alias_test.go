package main

import "testing"

func TestJSONStructAliases(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      string
		wantError bool
	}{
		{"root alias", `{"llmServices":[],"LLMSERVICES":[]}`, true},
		{"metadata alias", `{"llmServices":[{"metadata":{"name":"a","NAME":"b"}}]}`, true},
		{"condition alias", `{"llmServices":[{"status":{"conditions":[{"type":"a","Type":"b"}]}}]}`, true},
		{"reverse alias", `{"llmServices":[{"metadata":{"NAME":"b","name":"a"}}]}`, true},
		{"case-sensitive maps and live fields", `{"llmServices":[{"apiVersion":"serving.ckodex.com/v1","kind":"LLMInferenceService","metadata":{"name":"svc","namespace":"ns","labels":{"Key":"a","key":"b"},"annotations":{"Key":"a","key":"b"}},"data":{"KEY":"a","key":"b"},"spec":{"Name":"a","name":"b"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"Ready","lastTransitionTime":"2026-01-01T00:00:00Z"}]}}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			err := unmarshalUniqueJSON([]byte(tc.data), &in)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %t", err, tc.wantError)
			}
		})
	}
}

func TestJSONRegistrationStructAliases(t *testing.T) {
	var registration struct {
		ID  string `json:"spiffeId"`
		TTL int32  `json:"ttl"`
	}
	if err := unmarshalUniqueJSON([]byte(`{"spiffeId":"id","TTL":3600,"ttl":1}`), &registration); err == nil {
		t.Fatal("accepted aliases for registration TTL")
	}
}
