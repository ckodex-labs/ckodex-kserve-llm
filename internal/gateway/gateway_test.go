/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestBuildHTTPRoute_RoutePaths(t *testing.T) {
	llmSvc := baseLLMSvc("test-model")
	route := BuildHTTPRoute(llmSvc, nil)

	if route.Name != "test-model-httproute" {
		t.Errorf("name = %q, want %q", route.Name, "test-model-httproute")
	}
	if route.Namespace != "default" {
		t.Errorf("namespace = %q, want %q", route.Namespace, "default")
	}

	expectedPaths := []string{
		"/v2/health/",
		"/v2/models/",
		"/v2",
		"/v1/chat/completions",
		"/v1/embeddings",
		"/v1/models",
		// vLLM v0.25.1 Rust frontend endpoints
		"/version",
		"/server_info",
		"/v1/responses",
	}

	if len(route.Spec.Rules) != len(expectedPaths) {
		t.Fatalf("got %d rules, want %d", len(route.Spec.Rules), len(expectedPaths))
	}

	for i, expected := range expectedPaths {
		rule := route.Spec.Rules[i]
		if len(rule.Matches) == 0 || rule.Matches[0].Path == nil || rule.Matches[0].Path.Value == nil {
			t.Fatalf("rule[%d] has no path match", i)
		}
		if *rule.Matches[0].Path.Value != expected {
			t.Errorf("rule[%d] path = %q, want %q", i, *rule.Matches[0].Path.Value, expected)
		}
	}
}

func TestBuildHTTPRoute_BackendRef(t *testing.T) {
	llmSvc := baseLLMSvc("my-svc")
	route := BuildHTTPRoute(llmSvc, nil)

	for i, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) != 1 {
			t.Fatalf("rule[%d] has %d backendRefs, want 1", i, len(rule.BackendRefs))
		}
		ref := rule.BackendRefs[0]
		if string(ref.Name) != "my-svc" {
			t.Errorf("rule[%d] backendRef name = %q, want %q", i, ref.Name, "my-svc")
		}
		if ref.Port == nil || *ref.Port != 80 {
			t.Errorf("rule[%d] backendRef port = %v, want 80", i, ref.Port)
		}
	}
}

func TestBuildHTTPRoute_MultiNodeUsesKServePredictorService(t *testing.T) {
	llmSvc := baseLLMSvc("distributed")
	llmSvc.Spec.Model.URI = "pvc://weights"
	llmSvc.Spec.Worker = &servingv1alpha2.WorkerSpec{}

	route := BuildHTTPRoute(llmSvc, nil)
	if len(route.Spec.Rules) == 0 || len(route.Spec.Rules[0].BackendRefs) == 0 {
		t.Fatal("route has no backend")
	}
	got := route.Spec.Rules[0].BackendRefs[0].Name
	want := llmSvc.Name + "-predictor"
	if string(got) != want {
		t.Fatalf("backend name = %q, want %q", got, want)
	}
}

func TestBuildHTTPRoute_ParentRef_Managed(t *testing.T) {
	llmSvc := baseLLMSvc("test-model")
	route := BuildHTTPRoute(llmSvc, nil)

	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("got %d parentRefs, want 1", len(route.Spec.ParentRefs))
	}
	if string(route.Spec.ParentRefs[0].Name) != "test-model-gateway" {
		t.Errorf("parentRef name = %q, want %q", route.Spec.ParentRefs[0].Name, "test-model-gateway")
	}
}

func TestBuildHTTPRoute_Hostnames(t *testing.T) {
	llmSvc := baseLLMSvc("test-model")
	llmSvc.Spec.Router.Route.HTTPRoute = &servingv1alpha2.HTTPRouteSpec{
		Hostnames: []string{"model.example.com", "model.local"},
	}
	route := BuildHTTPRoute(llmSvc, nil)

	if len(route.Spec.Hostnames) != 2 {
		t.Fatalf("got %d hostnames, want 2", len(route.Spec.Hostnames))
	}
	if string(route.Spec.Hostnames[0]) != "model.example.com" {
		t.Errorf("hostname[0] = %q, want %q", route.Spec.Hostnames[0], "model.example.com")
	}
}

func TestBuildGRPCRoute_Methods(t *testing.T) {
	llmSvc := baseLLMSvc("test-model")
	route := BuildGRPCRoute(llmSvc)

	if route.Name != "test-model-grpcroute" {
		t.Errorf("name = %q, want %q", route.Name, "test-model-grpcroute")
	}

	expectedMethods := []string{
		"ModelInfer",
		"ServerLive",
		"ServerReady",
		"ModelReady",
		"ServerMetadata",
		"ModelMetadata",
	}

	if len(route.Spec.Rules) != len(expectedMethods) {
		t.Fatalf("got %d rules, want %d", len(route.Spec.Rules), len(expectedMethods))
	}

	for i, expected := range expectedMethods {
		rule := route.Spec.Rules[i]
		if len(rule.Matches) == 0 || rule.Matches[0].Method == nil || rule.Matches[0].Method.Method == nil {
			t.Fatalf("rule[%d] has no method match", i)
		}
		if *rule.Matches[0].Method.Method != expected {
			t.Errorf("rule[%d] method = %q, want %q", i, *rule.Matches[0].Method.Method, expected)
		}
		if rule.Matches[0].Method.Service == nil || *rule.Matches[0].Method.Service != "inference.GRPCInferenceService" {
			t.Errorf("rule[%d] service = %v, want %q", i, rule.Matches[0].Method.Service, "inference.GRPCInferenceService")
		}
	}
}

func TestBuildGRPCRoute_BackendRef(t *testing.T) {
	llmSvc := baseLLMSvc("my-svc")
	route := BuildGRPCRoute(llmSvc)

	for i, rule := range route.Spec.Rules {
		if len(rule.BackendRefs) != 1 {
			t.Fatalf("rule[%d] has %d backendRefs, want 1", i, len(rule.BackendRefs))
		}
		ref := rule.BackendRefs[0]
		if string(ref.Name) != "my-svc" {
			t.Errorf("rule[%d] backendRef name = %q, want %q", i, ref.Name, "my-svc")
		}
		if ref.Port == nil || *ref.Port != 8001 {
			t.Errorf("rule[%d] backendRef port = %v, want 8001", i, ref.Port)
		}
	}
}

func TestGatewayRef_Managed(t *testing.T) {
	llmSvc := baseLLMSvc("my-model")
	ref := GatewayRef(llmSvc)

	if string(ref.Name) != "my-model-gateway" {
		t.Errorf("managed ref name = %q, want %q", ref.Name, "my-model-gateway")
	}
	if ref.Namespace != nil {
		t.Errorf("managed ref should have nil namespace, got %v", ref.Namespace)
	}
}

func TestGatewayRef_Existing(t *testing.T) {
	llmSvc := baseLLMSvc("my-model")
	llmSvc.Spec.Router.Gateway.Managed = nil
	llmSvc.Spec.Router.Gateway.ExistingRef = &servingv1alpha2.GatewayRef{
		Name:      "shared-gateway",
		Namespace: "infra",
	}

	ref := GatewayRef(llmSvc)

	if string(ref.Name) != "shared-gateway" {
		t.Errorf("existing ref name = %q, want %q", ref.Name, "shared-gateway")
	}
	if ref.Namespace == nil || string(*ref.Namespace) != "infra" {
		t.Errorf("existing ref namespace = %v, want %q", ref.Namespace, "infra")
	}
}

func TestCommonLabels(t *testing.T) {
	llmSvc := baseLLMSvc("test")
	llmSvc.Spec.Model.Name = "org/model-name"

	labels := commonLabels(llmSvc)

	if labels["serving.ckodex.com/model"] != "org.model-name" {
		t.Errorf("model label = %q, want %q (slashes replaced with dots)",
			labels["serving.ckodex.com/model"], "org.model-name")
	}
}

// --- helpers ---

func baseLLMSvc(name string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://test/model",
				Name: "test-model",
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{
						GatewayClassName: "envoy",
					},
				},
			},
		},
	}
}
