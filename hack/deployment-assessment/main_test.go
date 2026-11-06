package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDeploymentCheck(t *testing.T) {
	data, err := os.ReadFile("testdata/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	valid := string(data)
	deployment, err := decodeDeployment(data)
	if err != nil {
		t.Fatal(err)
	}
	jsonDoc, err := json.Marshal(deployment)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		doc  string
		pass bool
	}{
		{"legacy example", valid, true},
		{"JSON deployment", string(jsonDoc), true},
		{"explicit false", strings.Replace(valid, "runAsNonRoot: true", "runAsNonRoot: false", 1), false},
		{"missing field", strings.Replace(valid, "runAsNonRoot: true", "runAsUser: 65532", 1), false},
		{"wrong kind", strings.Replace(valid, "kind: Deployment", "kind: Pod", 1), false},
		{"missing name", strings.Replace(valid, "name: demo-vllm", "name: ''", 1), false},
		{"multiple documents", valid + "\n---\n" + valid, false},
		{"duplicate key", strings.Replace(valid, "runAsNonRoot: true", "runAsNonRoot: true\n            runAsNonRoot: false", 1), false},
		{"missing containers", strings.Replace(valid, "containers:", "unknownContainers:", 1), false},
		{"init explicit true", valid + "      initContainers:\n        - name: setup\n          securityContext:\n            runAsNonRoot: true\n", true},
		{"init missing setting", valid + "      initContainers:\n        - name: setup\n", false},
		{"init explicit false", valid + "      initContainers:\n        - name: setup\n          securityContext:\n            runAsNonRoot: false\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment, err := decodeDeployment([]byte(test.doc))
			if err == nil {
				err = checkDeployment(deployment)
			}
			if (err == nil) != test.pass {
				t.Fatalf("pass = %t, error = %v", test.pass, err)
			}
		})
	}
}
