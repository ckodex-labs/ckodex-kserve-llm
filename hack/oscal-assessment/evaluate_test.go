package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestIdentityFixtures(t *testing.T) {
	for _, tc := range []struct {
		file string
		want bool
	}{
		{"testdata/spire-identity-valid.json", true},
		{"testdata/spire-identity-missing-registration.json", false},
	} {
		data, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		var in input
		if err := json.Unmarshal(data, &in); err != nil {
			t.Fatal(err)
		}
		in.Identity = identityContext{Mode: "direct"}
		got := evaluate(in, "ia-9")
		if len(got) != 1 || got[0].Passed != tc.want {
			t.Fatalf("%s: got %+v, want %t", tc.file, got, tc.want)
		}
		if tc.want {
			in.Entries = append(in.Entries, in.Entries[0])
			if evaluate(in, "ia-9")[0].Passed {
				t.Fatal("duplicate registration passed")
			}
		}
	}
}

func TestAllControlTransitions(t *testing.T) {
	var in input
	if err := json.Unmarshal([]byte(`{"llmServices":[{"metadata":{"name":"svc","namespace":"ns"},"status":{"conditions":[{"type":"Compliance-AU-2","status":"True","reason":"AuditPersistent"},{"type":"Compliance-SI-4","status":"True","reason":"OISSignalsActive"},{"type":"Compliance-SI-7","status":"True","reason":"IntegrityVerified"},{"type":"Compliance-SR-2","status":"True","reason":"ProvenanceVerified"}]}}],"networkPolicies":[{"metadata":{"name":"policy","namespace":"ns"}}]}`), &in); err != nil {
		t.Fatal(err)
	}
	for _, control := range []string{"ac-4", "au-2", "si-4", "si-7", "sr-2"} {
		if got := evaluate(in, control); len(got) != 1 || !got[0].Passed {
			t.Fatalf("%s: %+v", control, got)
		}
	}
	in.Policies = nil
	if evaluate(in, "ac-4")[0].Passed {
		t.Fatal("absent policy passed")
	}
	in.Services[0].Status.Conditions = nil
	for _, control := range []string{"au-2", "si-4", "si-7", "sr-2"} {
		if evaluate(in, control)[0].Passed {
			t.Fatalf("absent condition passed %s", control)
		}
	}
	in.Services = nil
	for _, got := range evaluate(in, "") {
		if got.Passed {
			t.Fatalf("empty services passed %s", got.Control)
		}
	}
}

func TestMissingServiceIdentityFails(t *testing.T) {
	in := input{Services: []object{{}}, Policies: []object{{}}}
	if evaluate(in, "ac-4")[0].Passed {
		t.Fatal("anonymous service and policy passed the namespace check")
	}
}

func TestNetworkPolicyCheckReportsPresenceOnly(t *testing.T) {
	var in input
	data := `{"llmServices":[{"metadata":{"name":"svc","namespace":"ns"}}],"networkPolicies":[{"metadata":{"name":"unrelated","namespace":"ns"},"spec":{"podSelector":{"matchLabels":{"app":"other"}},"ingress":[{}],"policyTypes":["Ingress"]}}]}`
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		t.Fatal(err)
	}
	got := evaluate(in, "ac-4")[0]
	if !got.Passed || !strings.Contains(got.Reason, "policy selection and enforcement not assessed") {
		t.Fatalf("presence-only result is misleading: %+v", got)
	}
	in.Policies[0].Metadata.Namespace = "other"
	got = evaluate(in, "ac-4")[0]
	if got.Passed || !strings.Contains(got.Reason, "no NetworkPolicy found in namespace ns") {
		t.Fatalf("policy in another namespace passed: %+v", got)
	}
}
