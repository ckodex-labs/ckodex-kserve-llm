/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ---- scheme helpers --------------------------------------------------------

func TestReconcileNetworkPolicies_DenyAllCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-deny-all-ingress", Namespace: "default"}, &policy))

	assert.Contains(t, policy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Empty(t, policy.Spec.Ingress, "deny-all must have empty ingress rules")
}

func TestReconcileNetworkPolicies_AllowGatewayCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-allow-gateway", Namespace: "default"}, &policy))

	require.Len(t, policy.Spec.Ingress, 1)
	require.Len(t, policy.Spec.Ingress[0].From, 2,
		"gateway ingress must allow both the workload-local scheduler and cross-namespace Envoy data plane")

	schedulerPeer := policy.Spec.Ingress[0].From[0]
	require.NotNil(t, schedulerPeer.PodSelector)
	assert.Nil(t, schedulerPeer.NamespaceSelector, "scheduler peer must remain namespace-local")
	assert.Equal(t, "scheduler", schedulerPeer.PodSelector.MatchLabels["serving.ckodex.com/role"])

	envoyPeer := policy.Spec.Ingress[0].From[1]
	require.NotNil(t, envoyPeer.NamespaceSelector,
		"Envoy Gateway runs outside the workload namespace, so an explicit namespace selector is mandatory")
	require.NotNil(t, envoyPeer.PodSelector)
	assert.Equal(t, map[string]string{
		"kubernetes.io/metadata.name": "envoy-gateway-system",
	}, envoyPeer.NamespaceSelector.MatchLabels)
	assert.Equal(t, map[string]string{
		"app.kubernetes.io/component":                    "proxy",
		"app.kubernetes.io/managed-by":                   "envoy-gateway",
		"gateway.envoyproxy.io/owning-gateway-name":      "llama3-gateway",
		"gateway.envoyproxy.io/owning-gateway-namespace": "default",
	}, envoyPeer.PodSelector.MatchLabels)

	ports := policy.Spec.Ingress[0].Ports
	require.Len(t, ports, 2)
	assert.Equal(t, int32(8000), ports[0].Port.IntVal)
	assert.Equal(t, int32(8001), ports[1].Port.IntVal)
}

func TestReconcileNetworkPolicies_ExistingGatewayIdentityScopesEnvoyPeer(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "tenant-a")
	svc.Spec.Router.Gateway.Managed = nil
	svc.Spec.Router.Gateway.ExistingRef = &servingv1alpha2.GatewayRef{
		Name:      "shared-ingress",
		Namespace: "gateway-infra",
	}

	np := &NetworkPolicyReconciler{
		Client:                    fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme:                    scheme,
		GatewayDataPlaneNamespace: "custom-envoy-system",
	}
	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-allow-gateway", Namespace: "tenant-a"}, &policy))

	require.Len(t, policy.Spec.Ingress, 1)
	require.Len(t, policy.Spec.Ingress[0].From, 2)
	envoyPeer := policy.Spec.Ingress[0].From[1]
	assert.Equal(t, "custom-envoy-system", envoyPeer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"])
	labels := envoyPeer.PodSelector.MatchLabels
	assert.Equal(t, "shared-ingress", labels["gateway.envoyproxy.io/owning-gateway-name"])
	assert.Equal(t, "gateway-infra", labels["gateway.envoyproxy.io/owning-gateway-namespace"])
}

// CRITICAL: Egress policy must permit DNS (53) so vLLM can resolve hostnames,
// and SPIRE Agent (8081) so pods can obtain SVIDs. Without these the model
// download and mTLS handshake both fail silently.
func TestReconcileNetworkPolicies_AllowEgressCreated_DNSAndSPIRE(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-egress-lockdown", Namespace: "default"}, &policy))

	assert.Contains(t, policy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
	require.Len(t, policy.Spec.Egress, 2, "must have DNS rule + SPIRE Agent rule")

	// DNS rule: ports 53 UDP and 53 TCP
	dnsPorts := policy.Spec.Egress[0].Ports
	require.Len(t, dnsPorts, 2)
	assert.Equal(t, int32(53), dnsPorts[0].Port.IntVal, "first rule must be port 53")

	// SPIRE rule: port 8081, scoped to spire-agent pod selector
	spirePorts := policy.Spec.Egress[1].Ports
	require.Len(t, spirePorts, 1)
	assert.Equal(t, int32(8081), spirePorts[0].Port.IntVal, "SPIRE egress must be port 8081")
}

func TestReconcileNetworkPolicies_FourPoliciesCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("mistral", "prod")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var list networkingv1.NetworkPolicyList
	require.NoError(t, np.List(context.Background(), &list))
	assert.Len(t, list.Items, 5)
}

func TestReconcileNetworkPolicies_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))
	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var list networkingv1.NetworkPolicyList
	require.NoError(t, np.List(context.Background(), &list))
	assert.Len(t, list.Items, 5, "idempotent — no duplicate policies created")
}

func TestReconcileNetworkPolicies_SanitizesInvalidCIDR(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("invalid-cidr", "default")
	svc.Spec.ToolSurface = &servingv1alpha2.ToolSurface{
		AllowedCIDRs: []string{"999.999.999.999/99", "invalid-ip", "10.0.0.0/24"},
	}

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	// Should not return error even with malformed CIDRs, should just skip them or fail gracefully
	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		client.ObjectKey{Name: "invalid-cidr-allow-tools", Namespace: "default"}, &policy))

	// Should only have the valid CIDR
	require.Len(t, policy.Spec.Egress, 1)
	assert.Equal(t, "10.0.0.0/24", policy.Spec.Egress[0].To[0].IPBlock.CIDR)
}

// ---- EbpfReconciler.ReconcileEbpfPolicy ------------------------------------
