package main

import "testing"

func TestSnapshotSourcePresence(t *testing.T) {
	cases := []struct {
		name, control, snapshot string
		wantError               bool
	}{
		{"services missing", "au-2", `{}`, true},
		{"services null", "au-2", `{"llmServices":null}`, true},
		{"services empty", "au-2", `{"llmServices":[]}`, false},
		{"services populated", "au-2", `{"llmServices":[{"metadata":{"name":"svc","namespace":"ns"}}]}`, false},
		{"policies missing", "ac-4", `{"llmServices":[]}`, true},
		{"policies null", "ac-4", `{"llmServices":[],"networkPolicies":null}`, true},
		{"policies empty", "ac-4", `{"llmServices":[],"networkPolicies":[]}`, false},
		{"entries missing", "ia-9", `{"llmServices":[]}`, true},
		{"entries null", "ia-9", `{"llmServices":[],"spireEntries":null}`, true},
		{"entries empty", "ia-9", `{"llmServices":[],"spireEntries":[]}`, false},
		{"full missing entries", "", `{"llmServices":[],"networkPolicies":[]}`, true},
		{"full missing policies", "", `{"llmServices":[],"spireEntries":[]}`, true},
		{"full empty", "", `{"llmServices":[],"networkPolicies":[],"spireEntries":[]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			if err := unmarshalUniqueJSON([]byte(tc.snapshot), &in); err != nil {
				t.Fatal(err)
			}
			if err := validateSnapshotSources(in, tc.control); (err != nil) != tc.wantError {
				t.Fatalf("validateSnapshotSources() = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestPolicyIdentities(t *testing.T) {
	for _, tc := range []struct {
		name      string
		policies  []object
		wantError bool
	}{
		{"empty list", nil, false},
		{"valid", []object{{Metadata: metadata{Name: "policy", Namespace: "ns"}}}, false},
		{"missing name", []object{{Metadata: metadata{Namespace: "ns"}}}, true},
		{"missing namespace", []object{{Metadata: metadata{Name: "policy"}}}, true},
		{"malformed alongside valid", []object{{Metadata: metadata{Name: "policy", Namespace: "ns"}}, {Metadata: metadata{Namespace: "ns"}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePolicyIdentities(tc.policies); (err != nil) != tc.wantError {
				t.Fatalf("validatePolicyIdentities() = %v, want error %v", err, tc.wantError)
			}
		})
	}
}
