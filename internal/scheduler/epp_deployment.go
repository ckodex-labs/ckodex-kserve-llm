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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func buildEPPDeployment(llmSvc *servingv1alpha2.LLMInferenceService, replicas int32, image string) *appsv1.Deployment {
	labels := eppLabels(llmSvc)
	name := eppName(llmSvc.Name)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: llmSvc.Namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       buildEPPPodSpec(llmSvc, image),
			},
		},
	}
}

func buildEPPPodSpec(llmSvc *servingv1alpha2.LLMInferenceService, image string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: EPPServiceAccountName,
		Containers:         []corev1.Container{buildEPPContainer(llmSvc, image)},
		Volumes:            []corev1.Volume{eppConfigVolume(llmSvc)},
	}
}

func buildEPPContainer(llmSvc *servingv1alpha2.LLMInferenceService, image string) corev1.Container {
	return corev1.Container{
		Name: "epp", Image: image, Args: eppArgs(llmSvc),
		Ports: []corev1.ContainerPort{
			{Name: "grpc", ContainerPort: EPPPort, Protocol: corev1.ProtocolTCP},
			{Name: "metrics", ContainerPort: EPPMetricsPort, Protocol: corev1.ProtocolTCP},
			{Name: "health", ContainerPort: EPPHealthPort, Protocol: corev1.ProtocolTCP},
		},
		VolumeMounts:   []corev1.VolumeMount{{Name: "plugins-config-volume", MountPath: "/config", ReadOnly: true}},
		ReadinessProbe: eppReadinessProbe(),
		Resources:      eppResources(),
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot: ptr.To(true), AllowPrivilegeEscalation: ptr.To(false), ReadOnlyRootFilesystem: ptr.To(true),
		},
	}
}

func eppArgs(llmSvc *servingv1alpha2.LLMInferenceService) []string {
	return []string{
		"--pool-name=" + llmSvc.Name, "--pool-namespace=" + llmSvc.Namespace,
		"--pool-group=inference.networking.k8s.io", "--config-file=/config/scheduler.yaml",
		fmt.Sprintf("--grpc-port=%d", EPPPort), fmt.Sprintf("--metrics-port=%d", EPPMetricsPort),
		fmt.Sprintf("--grpc-health-port=%d", EPPHealthPort), "--secure-serving=false",
		"--metrics-endpoint-auth=false", "--tracing=false",
	}
}

func eppReadinessProbe() *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{GRPC: &corev1.GRPCAction{Port: EPPHealthPort}}, PeriodSeconds: 5}
}

func eppResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: *parseQuantity("100m"), corev1.ResourceMemory: *parseQuantity("128Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: *parseQuantity("500m"), corev1.ResourceMemory: *parseQuantity("256Mi")},
	}
}

func parseQuantity(value string) *resource.Quantity {
	parsed := resource.MustParse(value)
	return &parsed
}

func eppConfigVolume(llmSvc *servingv1alpha2.LLMInferenceService) corev1.Volume {
	return corev1.Volume{
		Name: "plugins-config-volume",
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: llmSvc.Name + "-scheduler-config"},
		}},
	}
}

func (m *EPPManager) createOrUpdateDeployment(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, desired *appsv1.Deployment) error {
	name := eppName(llmSvc.Name)
	var existing appsv1.Deployment
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return m.Create(ctx, desired)
		}
		return err
	}
	if err := controllerutil.SetControllerReference(llmSvc, &existing, m.Scheme); err != nil {
		return fmt.Errorf("set existing epp deployment owner reference: %w", err)
	}
	existing.Labels = desired.Labels
	existing.Spec = desired.Spec
	return m.Update(ctx, &existing)
}
