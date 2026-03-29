/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// BuildGRPCRoute generates a GRPCRoute (Gateway API v1.4+ Standard Channel)
// with V2 gRPC method matching for GRPCInferenceService RPCs.
func BuildGRPCRoute(llmSvc *servingv1alpha2.LLMInferenceService) *gwapiv1.GRPCRoute {
	parentRef := GatewayRef(llmSvc)
	methodExact := gwapiv1.GRPCMethodMatchExact
	svcName := gwapiv1.ObjectName(llmSvc.Name)
	grpcPort := gwapiv1.PortNumber(8001)

	backendRef := gwapiv1.GRPCBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Name: svcName,
				Port: &grpcPort,
			},
		},
	}

	// V2 Open Inference Protocol gRPC service name
	serviceName := "inference.GRPCInferenceService"

	return &gwapiv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-grpcroute",
			Namespace: llmSvc.Namespace,
			Labels:    commonLabels(llmSvc),
		},
		Spec: gwapiv1.GRPCRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{parentRef},
			},
			Rules: []gwapiv1.GRPCRouteRule{
				// ModelInfer — core inference RPC
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ModelInfer"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
				// ServerLive — liveness check
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ServerLive"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
				// ServerReady — readiness check
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ServerReady"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
				// ModelReady — model-specific readiness
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ModelReady"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
				// ServerMetadata
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ServerMetadata"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
				// ModelMetadata
				{
					Matches: []gwapiv1.GRPCRouteMatch{
						{
							Method: &gwapiv1.GRPCMethodMatch{
								Type:    &methodExact,
								Service: &serviceName,
								Method:  strPtr("ModelMetadata"),
							},
						},
					},
					BackendRefs: []gwapiv1.GRPCBackendRef{backendRef},
				},
			},
		},
	}
}
