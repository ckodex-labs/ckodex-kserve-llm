package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestIA9RejectsAmbiguousRegistrationJSON(t *testing.T) {
	data, err := os.ReadFile("testdata/spire-identity-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	in.Identity = identityContext{Mode: "direct"}
	entry := in.Entries[0].Data["entry.json"]
	in.Entries[0].Data["entry.json"] = strings.Replace(entry, `"ttl":3600`, `"ttl":1,"ttl":3600`, 1)
	if got := evaluate(in, "ia-9"); len(got) != 1 || got[0].Passed {
		t.Fatalf("ambiguous registration passed: %+v", got)
	}
}

func TestControlRejectsContradictoryConditions(t *testing.T) {
	var in input
	data := `{"llmServices":[{"metadata":{"name":"llama3","namespace":"inference"},"status":{"conditions":[{"type":"Compliance-AU-2","status":"True","reason":"AuditPersistent"},{"type":"Compliance-AU-2","status":"False","reason":"AuditUnavailable"}]}}]}`
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		t.Fatal(err)
	}
	if got := evaluate(in, "au-2"); len(got) != 1 || got[0].Passed {
		t.Fatalf("contradictory conditions passed: %+v", got)
	}
}
