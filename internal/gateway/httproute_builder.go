/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
)

// BuildHTTPRoute generates an HTTPRoute with V2 protocol, OpenAI, and
// embedding path matchers for the given LLMInferenceService.
// It also injects high-priority Sandbox rules for any provided LLMLoraAdapters.
func BuildHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService, adapters []servingv1alpha2.LLMLoraAdapter) *gwapiv1.HTTPRoute {
	parentRef := GatewayRef(llmSvc)
	pathPrefix := gwapiv1.PathMatchPathPrefix
	pathExact := gwapiv1.PathMatchExact
	methodGet := gwapiv1.HTTPMethodGet
	methodPost := gwapiv1.HTTPMethodPost
	methodOptions := gwapiv1.HTTPMethodOptions
	svcName := gwapiv1.ObjectName(llmSvc.Name)
	if kserveintegration.RequiresMultiNode(llmSvc) {
		svcName = gwapiv1.ObjectName(kserveintegration.PredictorServiceName(llmSvc))
	}
	svcPort := gwapiv1.PortNumber(80)
	var backendGroup *gwapiv1.Group
	var backendKind *gwapiv1.Kind
	if llmSvc.Spec.Router.Scheduler != nil && SchedulerReady(llmSvc) {
		group := gwapiv1.Group("inference.networking.k8s.io")
		kind := gwapiv1.Kind("InferencePool")
		backendGroup = &group
		backendKind = &kind
		svcName = gwapiv1.ObjectName(llmSvc.Name)
		svcPort = gwapiv1.PortNumber(8000)
	}

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
				Group: backendGroup,
				Kind:  backendKind,
				Name:  svcName,
				Port:  &svcPort,
			},
		},
	}

	route := &gwapiv1.HTTPRoute{
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
				// Expose only inference and health paths, with the smallest method
				// set each path needs. vLLM's API key does not protect every endpoint.
				standardRules := []gwapiv1.HTTPRouteRule{
					// V2 Health endpoints: /v2/health/*
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/health/")}, Method: &methodGet},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// V2 Inference: /v2/models/{name}/infer
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v2/models/")}, Method: &methodPost},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// V2 Server metadata: /v2
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v2")}, Method: &methodGet},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// OpenAI-compatible: /v1/chat/completions
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}, Method: &methodPost},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/chat/completions")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// OpenAI-compatible text completion: /v1/completions
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/completions")}, Method: &methodPost},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/completions")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// Embeddings: /v1/embeddings
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}, Method: &methodPost},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/embeddings")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// OpenAI models list: /v1/models
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}, Method: &methodGet},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/models")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					// Responses creation and stored-response operations.
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/responses")}, Method: &methodPost},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathExact, Value: strPtr("/v1/responses")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
					{
						Matches: []gwapiv1.HTTPRouteMatch{
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v1/responses/")}, Method: &methodGet},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v1/responses/")}, Method: &methodPost},
							{Path: &gwapiv1.HTTPPathMatch{Type: &pathPrefix, Value: strPtr("/v1/responses/")}, Method: &methodOptions},
						},
						BackendRefs: []gwapiv1.HTTPBackendRef{backendRef},
						Timeouts:    timeouts,
						Filters:     inferenceResponseHeaders(),
					},
				}

				var rules []gwapiv1.HTTPRouteRule
				for _, adapter := range adapters {
					if adapter.Spec.Sandbox == nil || !adapter.Spec.Sandbox.Enable {
						continue
					}
					for _, standardRule := range standardRules {
						sandboxRule := standardRule
						sandboxRule.Matches = make([]gwapiv1.HTTPRouteMatch, len(standardRule.Matches))
						for index, match := range standardRule.Matches {
							sandboxRule.Matches[index] = match
							sandboxRule.Matches[index].Headers = append(
								append([]gwapiv1.HTTPHeaderMatch(nil), match.Headers...),
								gwapiv1.HTTPHeaderMatch{Name: "x-ckodex-adapter", Value: adapter.Spec.Sandbox.HeaderValue},
							)
						}
						rules = append(rules, sandboxRule)
					}
				}
				rules = append(rules, standardRules...)
				return rules
			}(),
		},
	}
	if llmSvc.Spec.Router.Scheduler != nil && !SchedulerReady(llmSvc) {
		blockHTTPRoute(route)
	}
	return route
}

// blockHTTPRoute retains matching rules while removing every forwarding path.
// Gateway API defines an error response for rules without filters or backends.
func blockHTTPRoute(route *gwapiv1.HTTPRoute) {
	for i := range route.Spec.Rules {
		route.Spec.Rules[i].BackendRefs = nil
		route.Spec.Rules[i].Filters = nil
		route.Spec.Rules[i].Timeouts = nil
	}
}

func inferenceResponseHeaders() []gwapiv1.HTTPRouteFilter {
	return []gwapiv1.HTTPRouteFilter{{
		Type: gwapiv1.HTTPRouteFilterResponseHeaderModifier,
		ResponseHeaderModifier: &gwapiv1.HTTPHeaderFilter{Set: []gwapiv1.HTTPHeader{
			{Name: "Cache-Control", Value: "no-store"},
			{Name: "X-Content-Type-Options", Value: "nosniff"},
			{Name: "Referrer-Policy", Value: "no-referrer"},
		}},
	}}
}

// BuildCanaryHTTPRoute generates an HTTPRoute that splits traffic between a
// canary service (weight%) and a stable base service ((100-weight)%).
func BuildCanaryHTTPRoute(llmSvc *servingv1alpha2.LLMInferenceService, adapters []servingv1alpha2.LLMLoraAdapter) *gwapiv1.HTTPRoute {
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
	route := BuildHTTPRoute(llmSvc, adapters)
	// Scheduler and canary cannot be combined; never route persisted invalid
	// combinations directly to a Service when admission was unavailable.
	if llmSvc.Spec.Router.Scheduler != nil {
		blockHTTPRoute(route)
		return route
	}
	for index := range route.Spec.Rules {
		sandbox := false
		for _, match := range route.Spec.Rules[index].Matches {
			for _, header := range match.Headers {
				if strings.EqualFold(string(header.Name), "x-ckodex-adapter") {
					sandbox = true
				}
			}
		}
		if sandbox {
			route.Spec.Rules[index].BackendRefs = []gwapiv1.HTTPBackendRef{canaryBackend}
			continue
		}
		route.Spec.Rules[index].BackendRefs = twoBackends
	}
	route.Labels["serving.ckodex.com/canary"] = "true"
	route.Labels["serving.ckodex.com/canary-weight"] = fmt.Sprintf("%d", canaryWeight)
	route.Labels["serving.ckodex.com/base-model"] = llmSvc.Spec.Canary.BaseModel
	route.Annotations = map[string]string{
		"serving.ckodex.com/canary-weight": fmt.Sprintf("%d", canaryWeight),
		"serving.ckodex.com/base-model":    llmSvc.Spec.Canary.BaseModel,
	}
	return route
}

// BuildRerankerHTTPRoute generates an HTTPRoute for a RerankerInferenceService.
// Exposes /rerank (Cohere-compatible) and /v1/rerank (OpenAI-compat alias).
// parentGatewayName selects the Gateway that should accept this route; when empty
// it defaults to "<svc.Name>-gateway" following the LLM controller convention.
func BuildRerankerHTTPRoute(svc *servingv1alpha2.RerankerInferenceService, parentGatewayName string) *gwapiv1.HTTPRoute {
	if parentGatewayName == "" {
		parentGatewayName = svc.Name + "-gateway"
	}
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
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{Name: gwapiv1.ObjectName(parentGatewayName)},
				},
			},
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

// SchedulerReady reports whether the SchedulerReady condition on the
// LLMInferenceService is True. BuildHTTPRoute uses this to avoid pointing the
// HTTPRoute at an InferencePool backend when the scheduler/EPP is not ready or
// has failed reconciliation. A stale condition is not current readiness.
func SchedulerReady(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	cond := meta.FindStatusCondition(llmSvc.Status.Conditions, servingv1alpha2.ConditionSchedulerReady)
	if cond == nil {
		return false
	}
	return cond.Status == metav1.ConditionTrue && cond.ObservedGeneration == llmSvc.Generation
}
