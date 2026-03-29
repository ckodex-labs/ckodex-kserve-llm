/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// BuildHTTPRoute generates an HTTPRoute with V2 protocol, OpenAI, and
// embedding path matchers for the given LLMInferenceService.
func BuildHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService) *gwapiv1.HTTPRoute {
	parentRef := GatewayRef(llmSvc)
	pathPrefix := gwapiv1.PathMatchPathPrefix
	pathExact := gwapiv1.PathMatchExact
	svcName := gwapiv1.ObjectName(llmSvc.Name)
	svcPort := gwapiv1.PortNumber(80)

	// Build hostnames from route spec
	var hostnames []gwapiv1.Hostname
	if llmSvc.Spec.Router.Route.HTTPRoute != nil {
		for _, h := range llmSvc.Spec.Router.Route.HTTPRoute.Hostnames {
			hostnames = append(hostnames, gwapiv1.Hostname(h))
		}
	}

	backendRef := gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Name: svcName,
				Port: &svcPort,
			},
		},
	}

	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-httproute",
			Namespace: llmSvc.Namespace,
			Labels:    commonLabels(llmSvc),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{parentRef},
			},
			Hostnames: hostnames,
			Rules: []gwapiv1.HTTPRouteRule{
				// V2 Health endpoints: /v2/health/*
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/health/")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
				// V2 Inference: /v2/models/{name}/infer
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/models/")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
				// V2 Server metadata: /v2
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v2")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
				// OpenAI-compatible: /v1/chat/completions
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
				// Embeddings: /v1/embeddings
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
				// OpenAI models list: /v1/models
				{
					Matches: []gwapiv1.HTTPRouteMatch{
						{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
				},
			},
		},
	}
}

// BuildCanaryHTTPRoute generates an HTTPRoute that splits traffic between a
// canary service (weight%) and a stable base service ((100-weight)%).
// It mirrors the same path matchers as BuildHTTPRoute so all OpenAI and V2
// paths participate in the canary split.
func BuildCanaryHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService) *gwapiv1.HTTPRoute {
	parentRef := GatewayRef(llmSvc)
	pathPrefix := gwapiv1.PathMatchPathPrefix
	pathExact := gwapiv1.PathMatchExact

	canaryWeight := llmSvc.Spec.Canary.Weight
	stableWeight := int32(100) - canaryWeight

	canaryPort := gwapiv1.PortNumber(80)
	stablePort := gwapiv1.PortNumber(80)

	canaryBackend := gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Name: gwapiv1.ObjectName(llmSvc.Name),
				Port: &canaryPort,
			},
			Weight: &canaryWeight,
		},
	}
	stableBackend := gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Name: gwapiv1.ObjectName(llmSvc.Spec.Canary.BaseModel),
				Port: &stablePort,
			},
			Weight: &stableWeight,
		},
	}

	twoBackends := []gwapiv1.HTTPBackendRef{canaryBackend, stableBackend}

	var hostnames []gwapiv1.Hostname
	if llmSvc.Spec.Router.Route.HTTPRoute != nil {
		for _, h := range llmSvc.Spec.Router.Route.HTTPRoute.Hostnames {
			hostnames = append(hostnames, gwapiv1.Hostname(h))
		}
	}

	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-httproute",
			Namespace: llmSvc.Namespace,
			Labels: func() map[string]string {
				labels := commonLabels(llmSvc)
				labels["serving.ckodex.com/canary"] = "true"
				labels["serving.ckodex.com/canary-weight"] = fmt.Sprintf("%d", canaryWeight)
				labels["serving.ckodex.com/base-model"] = llmSvc.Spec.Canary.BaseModel
				return labels
			}(),
			Annotations: map[string]string{
				"serving.ckodex.com/canary-weight": fmt.Sprintf("%d", canaryWeight),
				"serving.ckodex.com/base-model":    llmSvc.Spec.Canary.BaseModel,
			},
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{parentRef},
			},
			Hostnames: hostnames,
			Rules: []gwapiv1.HTTPRouteRule{
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/health/")}}}, BackendRefs: twoBackends},
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/models/")}}}, BackendRefs: twoBackends},
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v2")}}}, BackendRefs: twoBackends},
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}}}, BackendRefs: twoBackends},
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}}}, BackendRefs: twoBackends},
				{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}}}, BackendRefs: twoBackends},
			},
		},
	}
}

func strPtr(s string) *string { return &s }
