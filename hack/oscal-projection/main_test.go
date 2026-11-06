//go:build oskal

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectionPreservesResourceChecks(t *testing.T) {
	data := []byte(`[{"control":"ia-9","passed":true,"reason":"one registration matched"},{"control":"ac-4","passed":false,"reason":"no namespace policy"}]`)
	opts := options{Subject: "urn:example:assessment-scope", AssessmentPlan: "urn:example:assessment-plan", EvaluatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out, err := project(context.Background(), data, opts)
	if err != nil {
		t.Fatal(err)
	}
	var wrapper struct {
		Assessment struct {
			Import struct {
				Href string `json:"href"`
			} `json:"import-ap"`
			Results []struct {
				Description string `json:"description"`
				Reviewed    struct {
					Selections []map[string]json.RawMessage `json:"control-selections"`
				} `json:"reviewed-controls"`
				Observations []struct {
					Description string `json:"description"`
					Props       []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"props"`
				} `json:"observations"`
				Findings json.RawMessage `json:"findings"`
			} `json:"results"`
		} `json:"assessment-results"`
	}
	if err := json.Unmarshal(out, &wrapper); err != nil {
		t.Fatal(err)
	}
	if wrapper.Assessment.Import.Href != opts.AssessmentPlan || len(wrapper.Assessment.Results) != 1 {
		t.Fatal("incorrect plan or results")
	}
	result := wrapper.Assessment.Results[0]
	if !strings.Contains(result.Description, "unverified") || len(result.Observations) != 2 || len(result.Findings) != 0 {
		t.Fatal("result misrepresents check scope")
	}
	if _, ok := result.Reviewed.Selections[0]["include-all"]; ok {
		t.Fatal("claims all controls")
	}
	if result.Observations[0].Description != "one registration matched" || result.Observations[0].Props[0].Value != "passed" || result.Observations[1].Description != "no namespace policy" || result.Observations[1].Props[0].Value != "failed" {
		t.Fatalf("outcomes changed: %+v", result.Observations)
	}
	if strings.Contains(string(out), "epoch-composite-digest") || strings.Contains(string(out), "assurance-state") {
		t.Fatal("unverified assurance data emitted")
	}
}

func TestProjectionRejectsInvalidInputs(t *testing.T) {
	valid := []byte(`[{"control":"ia-9","passed":true,"reason":"matched"}]`)
	opts := options{Subject: "urn:example:scope", AssessmentPlan: "urn:example:plan", EvaluatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	for name, data := range map[string]string{
		"empty":          `[]`,
		"missing result": `[{"control":"ia-9","reason":"x"}]`,
		"missing reason": `[{"control":"ia-9","passed":true}]`,
		"unknown":        `[{"control":"other","passed":true,"reason":"x"}]`,
		"duplicate":      `[{"control":"ia-9","passed":true,"reason":"x"},{"control":"ia-9","passed":false,"reason":"y"}]`,
		"trailing":       `[] {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := project(context.Background(), []byte(data), opts); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	for name, option := range map[string]options{
		"subject": {Subject: "", AssessmentPlan: opts.AssessmentPlan, EvaluatedAt: opts.EvaluatedAt},
		"plan":    {Subject: opts.Subject, EvaluatedAt: opts.EvaluatedAt},
		"time":    {Subject: opts.Subject, AssessmentPlan: opts.AssessmentPlan},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := project(context.Background(), valid, option); err == nil {
				t.Fatal("missing input accepted")
			}
		})
	}
}

func TestDecodeChecksRejectsAmbiguousJSON(t *testing.T) {
	for name, data := range map[string]string{
		"false then true":  `[{"control":"ia-9","passed":false,"passed":true,"reason":"x"}]`,
		"true then false":  `[{"control":"ia-9","passed":true,"passed":false,"reason":"x"}]`,
		"field alias":      `[{"control":"ia-9","passed":false,"Passed":true,"reason":"x"}]`,
		"reverse alias":    `[{"CONTROL":"ia-9","control":"ac-4","passed":true,"reason":"x"}]`,
		"nested duplicate": `[{"control":"ia-9","passed":true,"reason":"x","other":{"a":1,"a":2}}]`,
		"deep nesting":     strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130),
		"oversize":         strings.Repeat(" ", (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeChecks([]byte(data)); err == nil {
				t.Fatal("ambiguous or unbounded input accepted")
			}
		})
	}
	if _, err := decodeChecks([]byte(`[{"control":"ia-9","passed":true,"reason":"ok"},{"control":"ac-4","passed":false,"reason":"absent"}]`)); err != nil {
		t.Fatalf("valid checks rejected: %v", err)
	}
}

func TestScaffoldDiscardsUnexpectedSDKFields(t *testing.T) {
	base := []byte(`{"extra":"injected","assessment-results":{"uuid":"c852672a-5f32-4c70-935e-bfbdc282cda7","metadata":{"published":"2026-01-01T00:00:00Z","last-modified":"2026-01-01T00:00:00Z","version":"1.0.0","oscal-version":"1.2.3","remarks":"injected"},"back-matter":{"remarks":"injected"},"results":[{"uuid":"4f3b5dc1-f2aa-4ae3-89e8-4f078f3b3cf3","findings":[{"title":"injected"}],"props":[{"name":"injected"}]}]}}`)
	doc, _, err := scaffoldFromSDK(base, options{Subject: "urn:example:scope", AssessmentPlan: "urn:example:plan"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "injected") || strings.Contains(string(out), "back-matter") || strings.Contains(string(out), "findings") {
		t.Fatalf("unexpected SDK fields retained: %s", out)
	}
}

func TestScaffoldRejectsInvalidSDKMetadata(t *testing.T) {
	base := `{"assessment-results":{"uuid":"c852672a-5f32-4c70-935e-bfbdc282cda7","metadata":{"published":"2026-01-01T00:00:00Z","last-modified":"2026-01-01T00:00:00Z","version":"1.0.0","oscal-version":"1.2.3"},"results":[{"uuid":"4f3b5dc1-f2aa-4ae3-89e8-4f078f3b3cf3"}]}}`
	for name, input := range map[string]string{
		"missing UUID":              strings.Replace(base, "c852672a-5f32-4c70-935e-bfbdc282cda7", "", 1),
		"invalid timestamp":         strings.Replace(base, "2026-01-01T00:00:00Z", "invalid", 1),
		"missing version":           strings.Replace(base, "1.0.0", "", 1),
		"unsupported OSCAL version": strings.Replace(base, "1.2.3", "1.0.0", 1),
		"missing result":            strings.Replace(base, `[{"uuid":"4f3b5dc1-f2aa-4ae3-89e8-4f078f3b3cf3"}]`, `[]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := scaffoldFromSDK([]byte(input), options{}); err == nil {
				t.Fatal("invalid SDK scaffold accepted")
			}
		})
	}
}
