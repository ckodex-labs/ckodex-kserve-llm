/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

const (
	// EPPImage is the Endpoint Picker Pod container image.
	// Pinned — :latest is a supply chain risk and air-gapped deployment blocker.
	// Must match OperatorConfig.Scheduler.Image default.
	EPPImage = "registry.k8s.io/gateway-api-inference-extension/epp@sha256:86c679b057298e68c6e65ff5603e92066d432e77b11f1f81f0a06399694810bc"
	// EPPPort is the EPP gRPC port (Gateway ExtensionRef).
	EPPPort int32 = 9002
	// EPPMetricsPort is the metrics/health port.
	EPPMetricsPort int32 = 9090
	// EPPHealthPort is the dedicated gRPC health port introduced by llm-d Router.
	EPPHealthPort int32 = 9003
	// EPPServiceAccountName is pre-provisioned by the platform/Helm profile in
	// each managed namespace. The operator does not create or mutate RBAC.
	EPPServiceAccountName = "ckodex-epp"
	// EPPServiceAccountLabel identifies the pre-provisioned identity contract.
	EPPServiceAccountLabel = "serving.ckodex.com/epp-rbac"
)

// EPPManager manages the Endpoint Picker Pod (EPP) deployment
// for KV-cache aware request routing per KServe v0.17 architecture.
type EPPManager struct {
	client.Client
	Scheme *runtime.Scheme
	Image  string
}

// +kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencemodelrewrites;inferenceobjectives,verbs=get;list;watch

// Reconcile creates/updates the EPP Deployment and Service.
func (m *EPPManager) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithValues("component", "epp")

	replicas := int32(1)
	if llmSvc.Spec.Router.Scheduler == nil {
		return fmt.Errorf("scheduler is not configured")
	}
	if llmSvc.Spec.Router.Scheduler.Replicas != nil {
		replicas = *llmSvc.Spec.Router.Scheduler.Replicas
	}
	if err := m.requireEPPServiceAccount(ctx, llmSvc); err != nil {
		return fmt.Errorf("validate pre-provisioned epp identity: %w", err)
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
	desired := buildEPPDeployment(llmSvc, replicas, m.eppImage())
	if err := controllerutil.SetControllerReference(llmSvc, desired, m.Scheme); err != nil {
		return err
	}
	return m.createOrUpdateDeployment(ctx, llmSvc, desired)
}

func (m *EPPManager) eppImage() string {
	if m.Image != "" {
		return m.Image
	}
	return EPPImage
}

func (m *EPPManager) requireEPPServiceAccount(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	var serviceAccount corev1.ServiceAccount
	key := types.NamespacedName{Name: EPPServiceAccountName, Namespace: llmSvc.Namespace}
	if err := m.Get(ctx, key, &serviceAccount); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("serviceaccount %q is not pre-provisioned in namespace %q; configure Helm managedNamespaces", EPPServiceAccountName, llmSvc.Namespace)
		}
		return err
	}
	if serviceAccount.Labels[EPPServiceAccountLabel] != "preprovisioned" {
		return fmt.Errorf("serviceaccount %q in namespace %q is missing label %s=preprovisioned", EPPServiceAccountName, llmSvc.Namespace, EPPServiceAccountLabel)
	}
	return nil
}

func (m *EPPManager) reconcileService(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	labels := eppLabels(llmSvc)
	name := eppName(llmSvc.Name)

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
	if err := controllerutil.SetControllerReference(llmSvc, &existing, m.Scheme); err != nil {
		return fmt.Errorf("set existing epp service owner reference: %w", err)
	}
	existing.Labels = labels
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
