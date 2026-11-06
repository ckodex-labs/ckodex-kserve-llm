//go:build oskal

// Command oscal-projection projects resource-check results into OSCAL.
// Run this command from the OSKAL module to use that checkout's SDK.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ckodex-labs/ckodex-oskal/pkg/oskal"
	"github.com/google/uuid"
)

type check struct {
	Control string `json:"control"`
	Passed  *bool  `json:"passed"`
	Reason  string `json:"reason"`
}

type options struct {
	Subject        string
	AssessmentPlan string
	EvaluatedAt    time.Time
	SourceDigest   string
}

func main() {
	var input, subject, plan, evaluated, sourceDigest string
	flag.StringVar(&input, "input", "", "resource-check results JSON")
	flag.StringVar(&subject, "subject", "", "subject URI identifying the assessed scope")
	flag.StringVar(&plan, "assessment-plan", "", "assessment plan URI or relative reference")
	flag.StringVar(&evaluated, "evaluated-at", "", "actual check evaluation time (RFC3339)")
	flag.StringVar(&sourceDigest, "oskal-source-digest", "", "OSKAL source content digest")
	flag.Parse()
	if err := run(input, subject, plan, evaluated, sourceDigest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, subject, plan, evaluated, sourceDigest string) error {
	if path == "" {
		return errors.New("--input is required")
	}
	t, err := time.Parse(time.RFC3339Nano, evaluated)
	if err != nil {
		return fmt.Errorf("--evaluated-at must be an RFC3339 timestamp: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read check results: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return fmt.Errorf("read check results: %w", err)
	}
	if len(data) > 1<<20 {
		return errors.New("check results exceed 1 MiB")
	}
	out, err := project(context.Background(), data, options{Subject: subject, AssessmentPlan: plan, EvaluatedAt: t, SourceDigest: sourceDigest})
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(append(out, '\n')); err != nil {
		return fmt.Errorf("write assessment: %w", err)
	}
	return nil
}

func project(ctx context.Context, data []byte, opts options) ([]byte, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	checks, err := decodeChecks(data)
	if err != nil {
		return nil, err
	}
	doc, result, err := assessmentScaffold(ctx, opts)
	if err != nil {
		return nil, err
	}
	populateResult(result, checks, data, opts)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode assessment: %w", err)
	}
	return out, nil
}

func validateOptions(opts options) error {
	subject, err := url.Parse(opts.Subject)
	if err != nil || subject == nil || subject.Scheme == "" || subject.Opaque == "" && subject.Host == "" {
		return errors.New("subject must be an absolute URI with an identity")
	}
	if strings.TrimSpace(opts.AssessmentPlan) == "" {
		return errors.New("assessment plan reference is required")
	}
	if _, err := url.Parse(opts.AssessmentPlan); err != nil {
		return fmt.Errorf("invalid assessment plan reference: %w", err)
	}
	if opts.EvaluatedAt.IsZero() {
		return errors.New("actual evaluation timestamp is required")
	}
	return nil
}

// assessmentScaffold retains only the SDK's structural identifiers and metadata.
func assessmentScaffold(ctx context.Context, opts options) (map[string]any, map[string]any, error) {
	base, err := oskal.NewProjector().ProjectAssessmentResults(ctx, oskal.SubjectRef{ID: opts.Subject}, nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("project assessment: %w", err)
	}
	return scaffoldFromSDK(base, opts)
}

func scaffoldFromSDK(base []byte, opts options) (map[string]any, map[string]any, error) {
	var sdk struct {
		Assessment struct {
			UUID     string `json:"uuid"`
			Metadata struct {
				Published    string `json:"published"`
				LastModified string `json:"last-modified"`
				Version      string `json:"version"`
				OscalVersion string `json:"oscal-version"`
			} `json:"metadata"`
			Results []struct {
				UUID string `json:"uuid"`
			} `json:"results"`
		} `json:"assessment-results"`
	}
	if err := json.Unmarshal(base, &sdk); err != nil {
		return nil, nil, fmt.Errorf("decode SDK projection: %w", err)
	}
	ar := sdk.Assessment
	if len(ar.Results) != 1 {
		return nil, nil, errors.New("SDK projection has unexpected results")
	}
	for _, id := range []string{ar.UUID, ar.Results[0].UUID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != strings.ToLower(id) {
			return nil, nil, errors.New("SDK projection has invalid UUID")
		}
	}
	for _, stamp := range []string{ar.Metadata.Published, ar.Metadata.LastModified} {
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, nil, fmt.Errorf("SDK projection has invalid metadata timestamp: %w", err)
		}
		if parsed.IsZero() {
			return nil, nil, errors.New("SDK projection has zero metadata timestamp")
		}
	}
	if strings.TrimSpace(ar.Metadata.Version) == "" || ar.Metadata.OscalVersion != "1.2.3" {
		return nil, nil, errors.New("SDK projection has invalid metadata version")
	}
	result := map[string]any{"uuid": ar.Results[0].UUID}
	doc := map[string]any{"assessment-results": map[string]any{
		"uuid":      ar.UUID,
		"metadata":  map[string]any{"title": "Resource check results for " + opts.Subject, "published": ar.Metadata.Published, "last-modified": ar.Metadata.LastModified, "version": ar.Metadata.Version, "oscal-version": ar.Metadata.OscalVersion},
		"import-ap": map[string]any{"href": opts.AssessmentPlan},
		"results":   []any{result},
	}}
	return doc, result, nil
}

func populateResult(result map[string]any, checks []check, data []byte, opts options) {
	result["title"] = "Resource check results for " + opts.Subject
	result["description"] = "Caller-supplied check results, subject, evaluation time, and assessment plan reference are unverified. These limited observations do not establish control compliance."
	props := []any{map[string]any{"name": "check-input-sha256", "value": fmt.Sprintf("%x", sha256.Sum256(data))}}
	if opts.SourceDigest != "" {
		props = append(props, map[string]any{"name": "oskal-source-content-digest", "value": opts.SourceDigest})
	}
	result["props"] = props
	result["start"] = opts.EvaluatedAt.UTC().Format(time.RFC3339Nano)
	result["end"] = opts.EvaluatedAt.UTC().Format(time.RFC3339Nano)
	selections := make([]any, 0, len(checks))
	observations := make([]any, 0, len(checks))
	for _, c := range checks {
		selections = append(selections, map[string]any{"control-id": c.Control})
		observations = append(observations, observation(c, opts.EvaluatedAt))
	}
	result["reviewed-controls"] = map[string]any{"control-selections": []any{map[string]any{"description": "Controls mapped to the resource checks; full control assessment is not asserted.", "include-controls": selections}}}
	result["observations"] = observations
}

func observation(c check, evaluatedAt time.Time) map[string]any {
	disposition := "failed"
	if *c.Passed {
		disposition = "passed"
	}
	return map[string]any{
		"uuid":        uuid.NewString(),
		"title":       "Resource check " + c.Control,
		"description": c.Reason,
		"collected":   evaluatedAt.UTC().Format(time.RFC3339Nano),
		"methods":     []string{"EXAMINE"},
		"types":       []string{"resource-check"},
		"props":       []any{map[string]any{"name": "check-result", "value": disposition}, map[string]any{"name": "control-id", "value": c.Control}},
	}
}

func decodeChecks(data []byte) ([]check, error) {
	if len(data) > 1<<20 {
		return nil, errors.New("check results exceed 1 MiB")
	}
	if err := validateUniqueKeys(data); err != nil {
		return nil, fmt.Errorf("decode check results: %w", err)
	}
	var checks []check
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&checks); err != nil {
		return nil, fmt.Errorf("decode check results: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing data in check results")
	}
	if len(checks) == 0 {
		return nil, errors.New("no resource checks supplied")
	}
	allowed := map[string]bool{"ac-4": true, "au-2": true, "ia-9": true, "si-4": true, "si-7": true, "sr-2": true}
	seen := make(map[string]bool)
	for _, c := range checks {
		if !allowed[c.Control] || seen[c.Control] || c.Passed == nil || strings.TrimSpace(c.Reason) == "" {
			return nil, fmt.Errorf("invalid or duplicate resource check %q", c.Control)
		}
		seen[c.Control] = true
	}
	return checks, nil
}

// validateUniqueKeys rejects ambiguous JSON before encoding/json can apply
// last-key-wins or case-insensitive struct field matching.
func validateUniqueKeys(data []byte) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := scanJSONValue(dec, 0, false); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing data in check results")
	}
	return nil
}

func scanJSONValue(dec *json.Decoder, depth int, checkObject bool) error {
	if depth > 128 {
		return errors.New("JSON nesting exceeds 128 levels")
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys, fields := map[string]bool{}, map[string]bool{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key := keyToken.(string)
			if keys[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = true
			if checkObject {
				for _, field := range []string{"control", "passed", "reason"} {
					if strings.EqualFold(key, field) {
						if fields[field] {
							return fmt.Errorf("multiple JSON keys target %q", field)
						}
						fields[field] = true
					}
				}
			}
			if err := scanJSONValue(dec, depth+1, false); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec, depth+1, depth == 0); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	_, err = dec.Token()
	return err
}
