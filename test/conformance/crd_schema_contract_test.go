/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package conformance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func TestGeneratedCRDSpecSchemasMatchRegisteredGoTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("register v1alpha2 API types: %v", err)
	}
	if err := servingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("register v1 API types: %v", err)
	}

	paths, err := filepath.Glob(filepath.Join("..", "..", "config", "crd", "serving*.yaml"))
	if err != nil {
		t.Fatalf("find generated CRDs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("generated CRD directory contains no serving CRDs")
	}
	sort.Strings(paths)
	for _, path := range paths {
		assertCRDSchemaFile(t, scheme, path)
	}
}

func assertCRDSchemaFile(t *testing.T, scheme *runtime.Scheme, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated CRD %s: %v", path, err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatalf("decode generated CRD %s: %v", path, err)
	}
	if len(crd.Spec.Versions) == 0 {
		t.Fatalf("generated CRD %s has no versions", path)
	}
	for _, version := range crd.Spec.Versions {
		assertCRDVersionSchema(t, scheme, crd.Spec.Group, crd.Spec.Names.Kind, version)
	}
}

func assertCRDVersionSchema(t *testing.T, scheme *runtime.Scheme, group, kind string, version apiextensionsv1.CustomResourceDefinitionVersion) {
	t.Helper()
	if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
		t.Fatalf("%s/%s %s has no OpenAPI schema", group, kind, version.Name)
	}
	specSchema, ok := version.Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		t.Fatalf("%s/%s %s schema has no spec property", group, kind, version.Name)
	}
	object, err := scheme.New(schema.GroupVersionKind{Group: group, Version: version.Name, Kind: kind})
	if err != nil {
		t.Fatalf("%s/%s %s is absent from the registered Go scheme: %v", group, kind, version.Name, err)
	}
	typeOf := reflect.TypeOf(object)
	if typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	specField, ok := typeOf.FieldByName("Spec")
	if !ok {
		t.Fatalf("%s/%s %s Go type has no Spec field", group, kind, version.Name)
	}
	want := jsonFieldNames(specField.Type)
	got := make([]string, 0, len(specSchema.Properties))
	for name := range specSchema.Properties {
		got = append(got, name)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s/%s %s spec schema differs from Go type: schema=%v go=%v", group, kind, version.Name, got, want)
	}
}

func jsonFieldNames(typeOf reflect.Type) []string {
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	fields := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.Split(typeOf.Field(index).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	return fields
}
