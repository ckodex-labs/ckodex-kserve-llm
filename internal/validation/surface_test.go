/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package validation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

func TestLLMInferenceServiceSurfaceContractMatchesSpecFields(t *testing.T) {
	for _, version := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "v1alpha2", typeOf: reflect.TypeOf(servingv1alpha2.LLMInferenceServiceSpec{})},
		{name: "v1", typeOf: reflect.TypeOf(servingv1.LLMInferenceServiceSpec{})},
	} {
		want := make([]string, 0, version.typeOf.NumField())
		for index := 0; index < version.typeOf.NumField(); index++ {
			field := version.typeOf.Field(index)
			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName != "" && jsonName != "-" {
				want = append(want, jsonName)
			}
		}
		got := make([]string, 0, len(want))
		for _, contract := range LLMInferenceServiceSurfaceContracts() {
			if contract.Version == version.name && !strings.Contains(contract.Path, ".") {
				got = append(got, contract.Path)
			}
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s surface inventory mismatch: got %v, want %v", version.name, got, want)
		}
	}
}

func TestNestedSurfaceContractMatchesReflectedFields(t *testing.T) {
	for _, version := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "v1alpha2", typeOf: reflect.TypeOf(servingv1alpha2.LLMInferenceServiceSpec{})},
		{name: "v1", typeOf: reflect.TypeOf(servingv1.LLMInferenceServiceSpec{})},
	} {
		want := reflectedNestedSurfacePaths(version.typeOf, version.name)
		got := make([]string, 0, len(want))
		for _, contract := range nestedSurfaceContracts() {
			if contract.Version == version.name {
				got = append(got, contract.Path)
			}
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s nested surface inventory mismatch: got %v, want %v", version.name, got, want)
		}
	}
}

func TestLLMInferenceServiceSurfaceContractHasExecutableDisposition(t *testing.T) {
	for _, contract := range LLMInferenceServiceSurfaceContracts() {
		if contract.Owner == "" {
			t.Errorf("%s %s has no owner", contract.Version, contract.Path)
		}
		switch contract.Disposition {
		case SurfaceRendered, SurfaceObserved:
		case SurfaceRefused:
			if contract.Owner != "ValidateLLMInferenceServiceSurface" && contract.Owner != "convertStableToAlpha" {
				t.Errorf("%s %s refusal is not bound to executable validation", contract.Version, contract.Path)
			}
		default:
			t.Errorf("%s %s has unknown disposition %q", contract.Version, contract.Path, contract.Disposition)
		}
	}
}

func TestObservedSurfaceFieldsHaveProductionReference(t *testing.T) {
	root := findModuleRoot(t)
	references := productionSpecFieldReferences(t, root)
	for _, contract := range LLMInferenceServiceSurfaceContracts() {
		if contract.Disposition != SurfaceObserved || strings.Contains(contract.Path, ".") {
			continue
		}
		fieldName := specGoFieldName(contract.Version, contract.Path)
		if !references[fieldName] {
			t.Errorf("%s %s is classified observed but has no production Spec.%s reference", contract.Version, contract.Path, fieldName)
		}
	}
}

var aggregateSurfacePaths = map[string]bool{
	"v1alpha2:model.storage.secretRef":                   true,
	"v1alpha2:template":                                  true,
	"v1alpha2:prefill.template":                          true,
	"v1alpha2:worker.template":                           true,
	"v1alpha2:router.scheduler.config.inline":            true,
	"v1alpha2:kvCache.transfer.env":                      true,
	"v1alpha2:kvCache.transfer.lmcache.engineRef":        true,
	"v1:model.storage.secretRef":                         true,
	"v1:template":                                        true,
	"v1:experimental.prefill.template":                   true,
	"v1:experimental.worker.template":                    true,
	"v1:router.scheduler.config.inline":                  true,
	"v1:experimental.kvCache.transfer.env":               true,
	"v1:experimental.kvCache.transfer.lmcache.engineRef": true,
}

func reflectedNestedSurfacePaths(typeOf reflect.Type, version string) []string {
	paths := []string{}
	collectNestedSurfacePaths(typeOf, "", version, &paths)
	return paths
}

func collectNestedSurfacePaths(typeOf reflect.Type, prefix, version string, paths *[]string) {
	typeOf = surfaceElementType(typeOf)
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if prefix == "" && directSurfaceDisposition(version, name) == SurfaceRefused {
			continue
		}
		if aggregateSurfacePaths[version+":"+path] || !isLocalSurfaceStruct(field.Type) {
			if prefix != "" {
				*paths = append(*paths, path)
			}
			continue
		}
		collectNestedSurfacePaths(field.Type, path, version, paths)
	}
}

func directSurfaceDisposition(version, path string) SurfaceDisposition {
	for _, contract := range llmInferenceServiceSurface {
		if contract.Version == version && contract.Path == path {
			return contract.Disposition
		}
	}
	return ""
}

func surfaceElementType(typeOf reflect.Type) reflect.Type {
	for typeOf.Kind() == reflect.Pointer || typeOf.Kind() == reflect.Slice {
		typeOf = typeOf.Elem()
	}
	return typeOf
}

func isLocalSurfaceStruct(typeOf reflect.Type) bool {
	typeOf = surfaceElementType(typeOf)
	return typeOf.Kind() == reflect.Struct && strings.HasPrefix(typeOf.PkgPath(), "github.com/ckodex-labs/kserve-llm-operator/api/")
}

func specGoFieldName(version, jsonName string) string {
	typeOf := reflect.TypeOf(servingv1alpha2.LLMInferenceServiceSpec{})
	if version == "v1" {
		typeOf = reflect.TypeOf(servingv1.LLMInferenceServiceSpec{})
	}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if strings.Split(field.Tag.Get("json"), ",")[0] == jsonName {
			return field.Name
		}
	}
	return ""
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found")
		}
		directory = parent
	}
}

func productionSpecFieldReferences(t *testing.T, root string) map[string]bool {
	t.Helper()
	references := map[string]bool{}
	set := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "zz_generated") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			spec, ok := selector.X.(*ast.SelectorExpr)
			if ok && spec.Sel.Name == "Spec" {
				references[selector.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk production source: %v", err)
	}
	return references
}

func TestValidateLLMInferenceServiceSurfaceRefusesUnimplementedValues(t *testing.T) {
	checkpoint := servingv1alpha2.QuantizationSpec{CheckpointPath: "/models/checkpoint"}
	service := &servingv1alpha2.LLMInferenceService{
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			BaseRefs:       []servingv1alpha2.ConfigReference{{Name: "defaults"}},
			AutoOptimize:   ptr.To(true),
			AllowedTenants: []string{"tenant-a"},
			SLO:            &servingv1alpha2.SLOSpec{TargetP99LatencyMs: 250},
			Quantization:   &checkpoint,
		},
	}

	errs := ValidateLLMInferenceServiceSurface(service)
	if len(errs) != 5 {
		t.Fatalf("got %d surface errors, want 5: %v", len(errs), errs)
	}
	for _, path := range []string{
		"spec.baseRefs",
		"spec.autoOptimize",
		"spec.allowedTenants",
		"spec.slo",
		"spec.quantization.checkpointPath",
	} {
		if !containsFieldPath(errs, path) {
			t.Errorf("surface errors do not contain %s: %v", path, errs)
		}
	}
}

func containsFieldPath(errs field.ErrorList, want string) bool {
	for _, err := range errs {
		if err.Field == want {
			return true
		}
	}
	return false
}
