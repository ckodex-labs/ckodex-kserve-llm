package main

import (
	"os"
	"strings"
	"testing"
)

func TestDeploymentSecurityCheckRequiresPerContainerSetting(t *testing.T) {
	data, err := os.ReadFile("testdata/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	base := string(data)
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{
			name: "pod-level setting does not replace container setting",
			doc: strings.Replace(strings.Replace(base,
				"    spec:\n      containers:",
				"    spec:\n      securityContext:\n        runAsNonRoot: true\n      containers:", 1),
				"            runAsNonRoot: true\n", "", 1),
		},
		{
			name: "empty container list",
			doc:  strings.Replace(base, "      containers:\n", "      containers: []\n", 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deployment, err := decodeDeployment([]byte(tc.doc))
			if err == nil {
				err = checkDeployment(deployment)
			}
			if err == nil {
				t.Fatal("deployment passed without the required container-level setting")
			}
		})
	}
}
