/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	// EPPImage is the Endpoint Picker Pod container image.
	// Pinned — :latest is a supply chain risk and air-gapped deployment blocker.
	// Must match OperatorConfig.Scheduler.Image default.
	EPPImage = "us-central1-docker.pkg.dev/k8s-staging-gateway-api/gateway-api-inference-extension/epp:main"
	// EPPPort is the EPP gRPC port (Gateway ExtensionRef).
	EPPPort int32 = 9002
	// EPPMetricsPort is the metrics/health port.
	EPPMetricsPort int32 = 9090
)

// EPPManager manages the Endpoint Picker Pod (EPP) deployment
// for KV-cache aware request routing per KServe v0.17 architecture.
type EPPManager struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates/updates the EPP Deployment and Service.
func (m *EPPManager) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "epp")

	replicas := int32(1)
	if llmSvc.Spec.Router.Scheduler.Replicas != nil {
		replicas = *llmSvc.Spec.Router.Scheduler.Replicas
	}

	// 1. Reconcile EPP Deployment
	if err := m.reconcileDeployment(ctx, llmSvc, replicas); err != nil {
		return fmt.Errorf("reconcile epp deployment: %w", err)
	}

	// 2. Reconcile EPP Service (ExtensionRef target)
	if err := m.reconcileService(ctx, llmSvc); err != nil {
		return fmt.Errorf("reconcile epp service: %w", err)
	}

	logger.Info("EPP scheduler reconciled", "replicas", replicas)
	return nil
}

func (m *EPPManager) reconcileDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, replicas int32) error {
	labels := eppLabels(llmSvc)
	name := llmSvc.Name + "-epp"

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "epp",
							Image: EPPImage,
							Args: []string{
								"--pool-name=" + llmSvc.Name,
								"--pool-namespace=" + llmSvc.Namespace,
								fmt.Sprintf("--grpc-port=%d", EPPPort),
								fmt.Sprintf("--metrics-port=%d", EPPMetricsPort),
							},
							Ports: []corev1.ContainerPort{
								{Name: "grpc", ContainerPort: EPPPort, Protocol: corev1.ProtocolTCP},
								{Name: "metrics", ContainerPort: EPPMetricsPort, Protocol: corev1.ProtocolTCP},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(EPPMetricsPort),
									},
								},
								PeriodSeconds: 5,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    *parseQuantity("100m"),
									corev1.ResourceMemory: *parseQuantity("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    *parseQuantity("500m"),
									corev1.ResourceMemory: *parseQuantity("256Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, m.Scheme); err != nil {
		return err
	}

	var existing appsv1.Deployment
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return m.Create(ctx, desired)
		}
		return err
	}
	existing.Spec = desired.Spec
	return m.Update(ctx, &existing)
}

func (m *EPPManager) reconcileService(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	labels := eppLabels(llmSvc)
	name := llmSvc.Name + "-epp"

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: llmSvc.Namespace, Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: EPPPort, TargetPort: intstr.FromInt32(EPPPort), Protocol: corev1.ProtocolTCP},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, m.Scheme); err != nil {
		return err
	}

	var existing corev1.Service
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return m.Create(ctx, desired)
		}
		return err
	}
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	return m.Update(ctx, &existing)
}

func eppLabels(llmSvc *servingv1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "epp",
		"app.kubernetes.io/instance":   llmSvc.Name + "-epp",
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/role":      "scheduler",
	}
}

func parseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
