/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

// Unit tests for RerankerInferenceServiceReconciler covering:
//   - --task score always present in container args
//   - Service port mapping: 80 → RerankerServerPort (8080)
//   - GetRerankerWellKnownConfig preset for bge-reranker-v2-m3

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// newRerankerSvc builds a minimal RerankerInferenceService for testing.
func newRerankerSvc(name, ns string) *servingv1alpha2.RerankerInferenceService {
	return &servingv1alpha2.RerankerInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: servingv1alpha2.RerankerInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://BAAI/bge-reranker-v2-m3",
				Name: "bge-reranker-v2-m3",
			},
		},
	}
}

// TestRerankerController_TaskScoreArgAlwaysPresent verifies --task score is always
// the first arg pair emitted regardless of other spec fields.
func TestRerankerController_TaskScoreArgAlwaysPresent(t *testing.T) {
	r := &RerankerInferenceServiceReconciler{}
	svc := newRerankerSvc("test-reranker", "default")

	c := r.buildContainer(svc)

	found := false
	for i := 0; i < len(c.Args)-1; i++ {
		if c.Args[i] == "--task" && c.Args[i+1] == "score" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("container args must contain '--task score', got %v", c.Args)
	}
}

// TestRerankerController_ServicePortMapping verifies the Service exposes port 80
// and targets RerankerServerPort (8080) on the pod.
func TestRerankerController_ServicePortMapping(t *testing.T) {
	r := &RerankerInferenceServiceReconciler{}
	svc := newRerankerSvc("test-reranker", "default")

	k8sSvc := r.buildService(svc)

	if len(k8sSvc.Spec.Ports) == 0 {
		t.Fatal("Service has no ports")
	}
	p := k8sSvc.Spec.Ports[0]
	if p.Port != 80 {
		t.Errorf("Service.Port = %d, want 80", p.Port)
	}
	targetPort := p.TargetPort.IntValue()
	if targetPort != servingv1alpha2.RerankerServerPort {
		t.Errorf("Service.TargetPort = %d, want %d", targetPort, servingv1alpha2.RerankerServerPort)
	}
}

// TestRerankerController_WellKnownPreset_BGEReranker verifies the bge-reranker-v2-m3
// preset returns MaxCandidates=100 and requests 1 GPU.
func TestRerankerController_WellKnownPreset_BGEReranker(t *testing.T) {
	preset := GetRerankerWellKnownConfig("hf://BAAI/bge-reranker-v2-m3")
	if preset == nil {
		t.Fatal("GetRerankerWellKnownConfig returned nil for bge-reranker-v2-m3")
	}
	if preset.MaxCandidates != 100 {
		t.Errorf("MaxCandidates = %d, want 100", preset.MaxCandidates)
	}
	if preset.Resources == nil {
		t.Fatal("Resources is nil in bge-reranker-v2-m3 preset")
	}
	gpuQty, ok := preset.Resources.Requests["nvidia.com/gpu"]
	if !ok {
		t.Fatal("nvidia.com/gpu missing from bge-reranker-v2-m3 preset resources")
	}
	if gpuQty.Value() != 1 {
		t.Errorf("nvidia.com/gpu = %s, want 1", gpuQty.String())
	}
}
