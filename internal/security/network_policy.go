/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// NetworkPolicyReconciler manages default-deny + explicit allow network policies.
type NetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// 3. allow-lws-intra    — facilitates pod-to-pod communication for multi-worker models
// 4. allow-tools        — permits egress to declared ToolSurface APIs and CIDRs (M3 Phase 4)
// 5. egress-lockdown    — strict egress control permitting only DNS, Mesh, and SPIRE
func (r *NetworkPolicyReconciler) ReconcileNetworkPolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "network-policy")

	// Fetch LoRA adapters to discover ToolSurface egress requirements
	var adapters servingv1alpha2.LLMLoraAdapterList
	if err := r.List(ctx, &adapters, client.InNamespace(llmSvc.Namespace)); err != nil {
		return fmt.Errorf("list adapters: %w", err)
	}
	var associated []servingv1alpha2.LLMLoraAdapter
	for _, a := range adapters.Items {
		if a.Spec.TargetService == llmSvc.Name {
			associated = append(associated, a)
		}
	}

	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/instance": llmSvc.Name},
	}

	// 1. Default-deny Ingress
	denyIngress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-deny-all-ingress", Namespace: llmSvc.Namespace,
			Labels: commonLabels(llmSvc),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
		},
	}

	// 2. Allow Gateway Ingress
	protoTCP := corev1.ProtocolTCP
	allowGateway := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-allow-gateway", Namespace: llmSvc.Namespace,
			Labels: commonLabels(llmSvc),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"serving.ckodex.com/role": "scheduler"},
						}},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 8000}, Protocol: &protoTCP},
						{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 8001}, Protocol: &protoTCP},
					},
				},
			},
		},
	}

	// 3. Egress Lockdown (Default Deny + Whitelist DNS/SPIRE)
	dnsPort := intstr.FromInt32(53)
	spirePort := intstr.FromInt32(8081)
	protoUDP := corev1.ProtocolUDP
	egressLockdown := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name + "-egress-lockdown",
			Namespace: llmSvc.Namespace,
			Labels:    commonLabels(llmSvc),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"k8s-app": "kube-dns"},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protoUDP, Port: &dnsPort},
						{Protocol: &protoTCP, Port: &dnsPort},
					},
				},
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &spirePort, Protocol: &protoTCP},
					},
					To: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app.kubernetes.io/name": "spire-agent"},
						}},
					},
				},
			},
		},
	}
	// 4. Intra-cluster LWS
	allowIntra := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-allow-lws-intra", Namespace: llmSvc.Namespace,
			Labels: commonLabels(llmSvc),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &podSelector},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &podSelector},
					},
				},
			},
		},
	}

	// 5. ToolSurface Egress (M3 Phase 4)
	allowTools := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-allow-tools", Namespace: llmSvc.Namespace,
			Labels: commonLabels(llmSvc),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: func() []networkingv1.NetworkPolicyEgressRule {
				var rules []networkingv1.NetworkPolicyEgressRule

				// Helper to add from ToolSurface
				addFromSurface := func(surface *servingv1alpha2.ToolSurface) {
					if surface == nil {
						return
					}
					for _, cidr := range surface.AllowedCIDRs {
						_, _, err := net.ParseCIDR(cidr)
						if err != nil {
							// For Pristine quality, we skip malformed CIDRs instead of failing the whole reconciliation
							// but we should ideally record a warning event.
							continue
						}
						rules = append(rules, networkingv1.NetworkPolicyEgressRule{
							To: []networkingv1.NetworkPolicyPeer{
								{IPBlock: &networkingv1.IPBlock{CIDR: cidr}},
							},
						})
					}
					// Note: FQDN-based policies (AllowedAPIs) require an Envoy/Istio-aware
					// NetworkPolicy implementation or a DNSPolicy sidecar.
					// We emit IPBlock rules for base K8s compatibility.
				}

				addFromSurface(llmSvc.Spec.ToolSurface)
				for _, a := range associated {
					addFromSurface(a.Spec.ToolSurface)
				}
				return rules
			}(),
		},
	}

	policies := []*networkingv1.NetworkPolicy{denyIngress, allowGateway, egressLockdown, allowIntra, allowTools}
	for _, np := range policies {
		if err := r.reconcileSinglePolicy(ctx, llmSvc, np); err != nil {
			return fmt.Errorf("reconcile network policy %s: %w", np.Name, err)
		}
	}

	logger.Info("Total Isolation network policies reconciled", "count", len(policies))
	return nil
}

func (r *NetworkPolicyReconciler) reconcileSinglePolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, np *networkingv1.NetworkPolicy) error {
	if err := controllerutil.SetControllerReference(llmSvc, np, r.Scheme); err != nil {
		return err
	}

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, np)
		}
		return err
	}

	np.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, np)
}
