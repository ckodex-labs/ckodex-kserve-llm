package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRegistrationRequiresExactWorkloadSelectorsAndSAN(t *testing.T) {
	fixture, err := os.ReadFile("testdata/spire-identity-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		identity  identityContext
		selectors []string
		sans      []string
		want      bool
	}{
		{"direct fixture", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local"}, true},
		{"reversed selectors", identityContext{Mode: "direct"}, []string{"k8s:sa:llama3", "k8s:ns:inference"}, []string{"llama3.inference.svc.cluster.local"}, true},
		{"vcluster", identityContext{Mode: "vcluster", HostNamespace: "host-shadow"}, []string{"k8s:ns:host-shadow", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local"}, true},
		{"wrong selector", identityContext{Mode: "direct"}, []string{"k8s:ns:other", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local"}, false},
		{"extra selector", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3", "k8s:sa:other"}, []string{"llama3.inference.svc.cluster.local"}, false},
		{"duplicate selector", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local"}, false},
		{"wrong SAN", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3"}, []string{"other.inference.svc.cluster.local"}, false},
		{"extra SAN", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local", "other.inference.svc.cluster.local"}, false},
		{"duplicate SAN", identityContext{Mode: "direct"}, []string{"k8s:ns:inference", "k8s:sa:llama3"}, []string{"llama3.inference.svc.cluster.local", "llama3.inference.svc.cluster.local"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in input
			if err := json.Unmarshal(fixture, &in); err != nil {
				t.Fatal(err)
			}
			var registration map[string]any
			if err := json.Unmarshal([]byte(in.Entries[0].Data["entry.json"]), &registration); err != nil {
				t.Fatal(err)
			}
			registration["selectors"] = tc.selectors
			registration["dnsSans"] = tc.sans
			data, err := json.Marshal(registration)
			if err != nil {
				t.Fatal(err)
			}
			in.Entries[0].Data["entry.json"] = string(data)
			in.Identity = tc.identity
			got := evaluate(in, "ia-9")
			if len(got) != 1 || got[0].Passed != tc.want {
				t.Fatalf("got %+v, want passed=%t", got, tc.want)
			}
		})
	}
}
