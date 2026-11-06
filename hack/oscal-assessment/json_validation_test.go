package main

import (
	"os"
	"strings"
	"testing"
)

func TestUniqueJSON(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      string
		wantError bool
	}{
		{"valid unknown Kubernetes fields", `{"llmServices":[{"apiVersion":"serving.ckodex.com/v1","metadata":{"name":"svc","namespace":"ns","managedFields":[{"fieldsV1":{"f:spec":{}}}]},"spec":{"replicas":1}}]}`, false},
		{"duplicate root", `{"llmServices":[],"llmServices":[]}`, true},
		{"duplicate nested unknown field", `{"ignored":{"objects":[{"a":1,"a":2}]}}`, true},
		{"escaped duplicate key", `{"name":1,"\u006eame":2}`, true},
		{"duplicate in separate objects is valid", `{"a":[{"k":1},{"k":2}]}`, false},
		{"multiple values", `{} {}`, true},
		{"malformed", `{"a":[}`, true},
		{"excessive nesting", strings.Repeat("[", 10002) + "0" + strings.Repeat("]", 10002), true},
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

func TestUniqueJSONIdentityFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/spire-identity-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var in input
	if err := unmarshalUniqueJSON(data, &in); err != nil {
		t.Fatal(err)
	}
	var registration any
	if err := unmarshalUniqueJSON([]byte(in.Entries[0].Data["entry.json"]), &registration); err != nil {
		t.Fatal(err)
	}
	if err := unmarshalUniqueJSON([]byte(`{"ttl":3600,"ttl":1}`), &registration); err == nil {
		t.Fatal("accepted duplicate embedded registration field")
	}
}

func TestUnambiguousCondition(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want bool
	}{
		{"one valid", `[{"type":"T","status":"True","reason":"R"}]`, true},
		{"duplicate conflicting", `[{"type":"T","status":"True","reason":"R"},{"type":"T","status":"False","reason":"R"}]`, false},
		{"duplicate identical", `[{"type":"T","status":"True","reason":"R"},{"type":"T","status":"True","reason":"R"}]`, false},
		{"different type", `[{"type":"T","status":"True","reason":"R"},{"type":"U","status":"False"}]`, true},
		{"missing", `[]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			data := `{"llmServices":[{"status":{"conditions":` + tc.data + `}}]}`
			if err := unmarshalUniqueJSON([]byte(data), &in); err != nil {
				t.Fatal(err)
			}
			if got := unambiguousCondition(in.Services[0], "T", "R"); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}
