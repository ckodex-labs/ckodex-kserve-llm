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
// It also injects high-priority Sandbox rules for any provided LLMLoraAdapters.
func BuildHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService, adapters []servingv1alpha2.LLMLoraAdapter) *gwapiv1.HTTPRoute {
	parentRef := GatewayRef(llmSvc)
	pathPrefix := gwapiv1.PathMatchPathPrefix
	pathExact := gwapiv1.PathMatchExact
	svcName := gwapiv1.ObjectName(llmSvc.Name)
	svcPort := gwapiv1.PortNumber(80)

	// Resilience preparation (M3 Phase 4)
	var timeouts *gwapiv1.HTTPRouteTimeouts
	if spec := llmSvc.Spec.Router.Route.HTTPRoute; spec != nil && spec.Resilience != nil {
		to := gwapiv1.Duration(spec.Resilience.Timeout)
		timeouts = &gwapiv1.HTTPRouteTimeouts{
			Request: &to,
		}
	}

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
			Rules: func() []gwapiv1.HTTPRouteRule {
				var rules []gwapiv1.HTTPRouteRule

				// 1. Inject Sandbox Rules (M3 Phase 3)
				for _, adapter := range adapters {
					if adapter.Spec.Sandbox != nil && adapter.Spec.Sandbox.Enable {
						headerValue := adapter.Spec.Sandbox.HeaderValue
						rules = append(rules, gwapiv1.HTTPRouteRule{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/")},
									Headers: []gwapiv1.HTTPHeaderMatch{
										{
											Name:  "x-ckodex-adapter",
											Value: headerValue,
										},
									},
								},
							},
							BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
							Timeouts:    timeouts,
						})
					}
				}

				// 2. Standard Protocol Rules
				standardRules := []gwapiv1.HTTPRouteRule{
					// V2 Health endpoints: /v2/health/*
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/health/")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// V2 Inference: /v2/models/{name}/infer
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/models/")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// V2 Server metadata: /v2
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v2")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// OpenAI-compatible: /v1/chat/completions
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// Embeddings: /v1/embeddings
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// OpenAI models list: /v1/models
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// vLLM v0.23.0 Rust frontend: metadata endpoints
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/version")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/server_info")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
					// vLLM v0.23.0 Responses API (Anthropic Messages-compatible endpoint)
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v1/responses")}},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
					},
				}

				// Apply Retries via Filter (Implementation specific or Standard if supported)
				// For Envoy Gateway (standard in many stacks), we use an extension or
				// just ensure the base rules are correct. Standard Gateway API v1.1+
				// doesn't have a cross-platform 'Retry' filter yet, so we'll stick to Timeouts
				// which are standard in v1.1.

				rules = append(rules, standardRules...)
				return rules
			}(),
		},
	}
}

// BuildCanaryHTTPRoute generates an HTTPRoute that splits traffic between a
// canary service (weight%) and a stable base service ((100-weight)%).
func BuildCanaryHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService, adapters []servingv1alpha2.LLMLoraAdapter) *gwapiv1.HTTPRoute {
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
			Rules: func() []gwapiv1.HTTPRouteRule {
				// Resilience preparation (M3 Phase 4)
				var timeouts *gwapiv1.HTTPRouteTimeouts
				if spec := llmSvc.Spec.Router.Route.HTTPRoute; spec != nil && spec.Resilience != nil {
					to := gwapiv1.Duration(spec.Resilience.Timeout)
					timeouts = &gwapiv1.HTTPRouteTimeouts{
						Request: &to,
					}
				}

				var rules []gwapiv1.HTTPRouteRule
				// Sandbox rules also apply to Canary services (routed to 'this' backend)
				for _, adapter := range adapters {
					if adapter.Spec.Sandbox != nil && adapter.Spec.Sandbox.Enable {
						rules = append(rules, gwapiv1.HTTPRouteRule{
							Matches: []gwapiv1.HTTPRouteMatch{
								{
									Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/")},
									Headers: []gwapiv1.HTTPHeaderMatch{{Name: "x-ckodex-adapter", Value: adapter.Spec.Sandbox.HeaderValue}},
								},
							},
							BackendRefs: []gwapiv1.HTTPBackendRef{canaryBackend}, // Explicitly route to canary backend
							Timeouts:    timeouts,
						})
					}
				}
				rules = append(rules, []gwapiv1.HTTPRouteRule{
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/health/")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/models/")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v2")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
					{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}}}, BackendRefs: twoBackends, Timeouts: timeouts},
				}...)
				return rules
			}(),
		},
	}
}

// BuildRerankerHTTPRoute generates an HTTPRoute for a RerankerInferenceService.
// Exposes /rerank (Cohere-compatible) and /v1/rerank (OpenAI-compat alias).
func BuildRerankerHTTPRoute(svc *servingv1alpha2.RerankerInferenceService) *gwapiv1.HTTPRoute {
	pathExact := gwapiv1.PathMatchExact
	svcPort := gwapiv1.PortNumber(80)
	backend := gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Name: gwapiv1.ObjectName(svc.Name),
				Port: &svcPort,
			},
		},
	}
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name + "-httproute",
			Namespace: svc.Namespace,
			Labels:    map[string]string{"serving.ckodex.com/reranker": svc.Name},
		},
		Spec: gwapiv1.HTTPRouteSpec{
			Rules: []gwapiv1.HTTPRouteRule{
				{
					Matches:     []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/rerank")}}},
					BackendRefs: []gwapiv1.HTTPBackendRef{backend},
				},
				{
					Matches:     []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/rerank")}}},
					BackendRefs: []gwapiv1.HTTPBackendRef{backend},
				},
			},
		},
	}
}

func strPtr(s string) *string { return &s }
