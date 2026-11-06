package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestIdentityContextValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context identityContext
		valid   bool
	}{
		{"missing", identityContext{}, false},
		{"unknown", identityContext{Mode: "auto"}, false},
		{"direct", identityContext{Mode: "direct"}, true},
		{"direct with host", identityContext{Mode: "direct", HostNamespace: "host"}, false},
		{"vcluster missing host", identityContext{Mode: "vcluster"}, false},
		{"vcluster invalid host", identityContext{Mode: "vcluster", HostNamespace: "INVALID"}, false},
		{"vcluster", identityContext{Mode: "vcluster", HostNamespace: "host-shadow"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.context.validate() == nil) != tc.valid {
				t.Fatalf("unexpected validation outcome for %+v", tc.context)
			}
		})
	}
}

func TestIdentitySelectorUsesCallerContext(t *testing.T) {
	data, err := os.ReadFile("testdata/spire-identity-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	entry := in.Entries[0]
	entry.Data["entry.json"] = strings.Replace(entry.Data["entry.json"], "k8s:ns:inference", "k8s:ns:host-shadow", 1)
	in.Entries[0] = entry
	for _, tc := range []struct {
		name    string
		context identityContext
		want    bool
	}{
		{"missing context", identityContext{}, false},
		{"direct rejects host", identityContext{Mode: "direct"}, false},
		{"vcluster accepts physical selector", identityContext{Mode: "vcluster", HostNamespace: "host-shadow"}, true},
		{"vcluster rejects wrong physical selector", identityContext{Mode: "vcluster", HostNamespace: "other-host"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in.Identity = tc.context
			got := evaluate(in, "ia-9")
			if len(got) != 1 || got[0].Passed != tc.want {
				t.Fatalf("unexpected result: %+v", got)
			}
			if tc.context.Mode != "" && !strings.Contains(got[0].Reason, "caller-asserted identity mode=") {
				t.Fatalf("result omits context: %+v", got[0])
			}
		})
	}
}

func TestRegistrationNamespaceAndTTL(t *testing.T) {
	data, err := os.ReadFile("testdata/spire-identity-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		namespace string
		ttl       string
		want      bool
	}{
		{"valid", "spire", "3600", true},
		{"wrong namespace", "other", "3600", false},
		{"missing namespace", "", "3600", false},
		{"fractional ttl", "spire", "1.5", false},
		{"zero ttl", "spire", "0", false},
		{"excessive ttl", "spire", "3601", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			if err := json.Unmarshal(data, &in); err != nil {
				t.Fatal(err)
			}
			in.Identity = identityContext{Mode: "direct"}
			in.Entries[0].Metadata.Namespace = tc.namespace
			entry := in.Entries[0].Data["entry.json"]
			in.Entries[0].Data["entry.json"] = strings.Replace(entry, `"ttl":3600`, `"ttl":`+tc.ttl, 1)
			got := evaluate(in, "ia-9")
			if len(got) != 1 || got[0].Passed != tc.want {
				t.Fatalf("unexpected result: %+v, want %t", got, tc.want)
			}
		})
	}
}
