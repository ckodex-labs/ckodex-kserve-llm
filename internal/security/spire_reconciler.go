/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package security implements SPIFFE/SPIRE, Vault, OPA, and network policy management.
package security

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	// SPIREAgentImage is the SPIRE Agent container image.
	SPIREAgentImage = "ghcr.io/spiffe/spire-agent:1.14.0"
	// SPIREServerImage is the SPIRE Server container image.
	SPIREServerImage = "ghcr.io/spiffe/spire-server:1.14.0"
	// SPIFFEHelperImage is the SPIFFE helper sidecar image that manages SVID rotation.
	SPIFFEHelperImage = "ghcr.io/spiffe/spiffe-helper:0.9.0"
	// SPIFFETrustDomain is the trust domain for SPIFFE IDs.
	SPIFFETrustDomain = "ckodex.com"
	// SPIFFEWorkloadAPIPath is the mount path for the SPIFFE Workload API socket (CSI driver).
	SPIFFEWorkloadAPIPath = "/run/spiffe/workload"
)

// SPIREReconciler manages native SPIFFE/SPIRE infrastructure.
type SPIREReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SPIFFEIDForService generates a SPIFFE ID for a given service.
// Format: spiffe://ckodex.com/ns/{namespace}/sa/{serviceaccount}/model/{modelname}
func SPIFFEIDForService(namespace, serviceAccount, modelName string) string {
	return fmt.Sprintf("spiffe://%s/ns/%s/sa/%s/model/%s",
		SPIFFETrustDomain, namespace, serviceAccount, modelName)
}

// ReconcileSPIRE ensures SPIRE Server and Agent are deployed.
func (r *SPIREReconciler) ReconcileSPIRE(ctx context.Context, namespace string) error {
	logger := log.FromContext(ctx).WithValues("component", "spire")

	// Reconcile SPIRE Server StatefulSet
	if err := r.reconcileSPIREServer(ctx, namespace); err != nil {
		return fmt.Errorf("reconcile spire server: %w", err)
	}

	// Reconcile SPIRE Agent DaemonSet
	if err := r.reconcileSPIREAgent(ctx, namespace); err != nil {
		return fmt.Errorf("reconcile spire agent: %w", err)
	}

	logger.Info("SPIRE infrastructure reconciled")
	return nil
}

func (r *SPIREReconciler) reconcileSPIREServer(ctx context.Context, namespace string) error {
	name := "spire-server"
	labels := map[string]string{
		"app.kubernetes.io/name":       "spire-server",
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(int32(1)),
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "spire-server",
							Image: SPIREServerImage,
							Ports: []corev1.ContainerPort{{Name: "grpc", ContainerPort: 8081}},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
							},
						},
					},
				},
			},
		},
	}

	var existing appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}
	return nil // Don't update if exists
}

func (r *SPIREReconciler) reconcileSPIREAgent(ctx context.Context, namespace string) error {
	name := "spire-agent"
	labels := map[string]string{
		"app.kubernetes.io/name":       "spire-agent",
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	desired := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "spire-agent",
							Image: SPIREAgentImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
							},
						},
					},
					HostNetwork: true, // Required for node attestation
				},
			},
		},
	}

	var existing appsv1.DaemonSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}
	return nil
}

// NetworkPolicyReconciler manages default-deny + explicit allow network policies.
type NetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ReconcileNetworkPolicies creates default-deny + explicit allow network policies.
// Three policies are reconciled per LLMInferenceService:
//  1. deny-all-ingress  — blocks all ingress by default (whitelist model)
//  2. allow-gateway     — permits ingress from the EPP scheduler (ports 8000/8001)
//  3. allow-egress      — permits egress to DNS (53 UDP+TCP) and SPIRE Agent (8081)
//
// Without policy 3, vLLM pods cannot resolve hostnames (model download fails)
// and cannot obtain SPIFFE SVIDs from the SPIRE Agent.
func (np *NetworkPolicyReconciler) ReconcileNetworkPolicies(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "network-policy")

	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/instance": llmSvc.Name},
	}

	// 1. Default-deny all ingress.
	deny := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-deny-all", Namespace: llmSvc.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
		},
	}

	// 2. Allow ingress from the EPP scheduler (ports 8000 = HTTP inference, 8001 = metrics).
	protoTCP := corev1.ProtocolTCP
	allow := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-allow-gateway", Namespace: llmSvc.Namespace,
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

	// 3. Egress: DNS resolution + SPIRE Agent.
	// Without DNS egress vLLM cannot resolve huggingface.co or any external hostname.
	// Without SPIRE egress the pod cannot obtain a SVID for mTLS.
	protoUDP := corev1.ProtocolUDP
	dnsPort := intstr.FromInt32(53)
	spirePort := intstr.FromInt32(8081)
	egress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: llmSvc.Name + "-allow-egress", Namespace: llmSvc.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// DNS over UDP (standard) and TCP (large responses / DoT fallback)
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: &dnsPort, Protocol: &protoUDP},
						{Port: &dnsPort, Protocol: &protoTCP},
					},
				},
				{
					// SPIRE Agent gRPC endpoint (Workload API)
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

	for _, policy := range []*networkingv1.NetworkPolicy{deny, allow, egress} {
		if err := controllerutil.SetControllerReference(llmSvc, policy, np.Scheme); err != nil {
			return fmt.Errorf("set owner reference on %s: %w", policy.Name, err)
		}
		var existing networkingv1.NetworkPolicy
		if err := np.Get(ctx, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}, &existing); err != nil {
			if apierrors.IsNotFound(err) {
				if createErr := np.Create(ctx, policy); createErr != nil {
					return fmt.Errorf("create network policy %s: %w", policy.Name, createErr)
				}
			}
		}
	}

	logger.Info("network policies reconciled", "policies", 3)
	return nil
}

// InjectSidecar injects the SPIFFE Workload API volume and SPIFFE helper sidecar into a PodSpec.
//
// Three changes are made:
//  1. CSI volume "spiffe-workload-api" is appended (driver: spiffe.csi.spiffe.io).
//     The driver mounts the SPIFFE Workload API UNIX socket at SPIFFEWorkloadAPIPath.
//  2. The primary container (index 0, the vLLM container) gets a read-only volume mount
//     at SPIFFEWorkloadAPIPath so it can verify SVIDs for mTLS.
//  3. The "spiffe-sidecar" container is appended. It runs spiffe-helper which:
//     - Connects to the SPIRE Agent via the Workload API socket
//     - Obtains the workload SVID and trust bundle
//     - Writes PEM files to /run/spiffe/certs/ and rotates them before expiry
//     vLLM (and Envoy sidecars) read the cert/key files for mTLS connections.
func (r *SPIREReconciler) InjectSidecar(podSpec *corev1.PodSpec, llmSvc *servingv1alpha2.LLMInferenceService) {
	// 1. CSI volume — the driver exposes the Workload API socket without host path mounts.
	socketVolume := corev1.Volume{
		Name: "spiffe-workload-api",
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   "spiffe.csi.spiffe.io",
				ReadOnly: ptr.To(true),
			},
		},
	}
	podSpec.Volumes = append(podSpec.Volumes, socketVolume)

	// Shared volume mount used by both the primary container and the sidecar.
	spiffeMount := corev1.VolumeMount{
		Name:      "spiffe-workload-api",
		MountPath: SPIFFEWorkloadAPIPath,
		ReadOnly:  true,
	}

	// 2. Mount SPIFFE socket into the vLLM container (index 0).
	if len(podSpec.Containers) > 0 {
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, spiffeMount)
		// Advertise the socket path so vLLM Python code can locate it without hardcoding.
		podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
			Name:  "SPIFFE_ENDPOINT_SOCKET",
			Value: "unix://" + SPIFFEWorkloadAPIPath + "/spiffe.sock",
		})
	}

	// 3. SPIFFE helper sidecar — manages SVID rotation and writes PEM files.
	spiffeID := SPIFFEIDForService(llmSvc.Namespace, llmSvc.Name, llmSvc.Name)
	sidecar := corev1.Container{
		Name:  "spiffe-sidecar",
		Image: SPIFFEHelperImage,
		Args:  []string{"-config", "/etc/spiffe-helper/helper.conf"},
		Env: []corev1.EnvVar{
			{
				Name:  "SPIFFE_ENDPOINT_SOCKET",
				Value: "unix://" + SPIFFEWorkloadAPIPath + "/spiffe.sock",
			},
			// Expose the generated SPIFFE ID for observability.
			{Name: "CKODEX_SPIFFE_ID", Value: spiffeID},
		},
		VolumeMounts: []corev1.VolumeMount{spiffeMount},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
			// Must be false: helper writes cert/key PEM files to /run/spiffe/certs/
			ReadOnlyRootFilesystem: ptr.To(false),
		},
	}
	podSpec.Containers = append(podSpec.Containers, sidecar)
}

// ReconcileSecurityPolicy (Placeholder for OPA/Admission)
func (r *SPIREReconciler) ReconcileSecurityPolicy(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	return nil
}
