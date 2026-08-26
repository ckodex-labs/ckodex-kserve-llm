/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

func TestReconcileGovernanceEvidence_SR2FailsClosedWithoutVerifiedArtifacts(t *testing.T) {
	gr := &evidence.GovernanceReconciler{
		AirGappedMode:      true,
		LocalCosignKeyPath: "/etc/cosign/cosign.pub",
	}
	llmSvc := baseLLMInferenceService()

	err := gr.Reconcile(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	sr2 := meta.FindStatusCondition(llmSvc.Status.Conditions, "Compliance-SR-2")
	require.NotNil(t, sr2)
	assert.Equal(t, metav1.ConditionFalse, sr2.Status)
	assert.Equal(t, "OfflineVerificationPending", sr2.Reason)

	si7 := meta.FindStatusCondition(llmSvc.Status.Conditions, "Compliance-SI-7")
	require.NotNil(t, si7)
	assert.Equal(t, metav1.ConditionFalse, si7.Status)
	assert.Equal(t, "IntegrityUnverified", si7.Reason)
}

func verifiedRuntimeRecord() provenance.RuntimeVerificationRecord {
	return provenance.RuntimeVerificationRecord{
		Subject:             "oci://registry.example.com/model@sha256:abc",
		Scheme:              "oci",
		SignatureVerified:   true,
		AttestationVerified: true,
		SBOMVerified:        true,
		SignatureDigest:     "sha256:abc",
		AttestationURI:      "oci://registry.example.com/model@sha256:abc#attestation:slsaprovenance1",
		SBOMDigest:          "sha256:def",
		VerifiedAt:          "2026-05-11T12:00:00Z",
	}
}

func verifiedModelPod(t *testing.T, serviceName string) *corev1.Pod {
	t.Helper()
	message, err := json.Marshal(verifiedRuntimeRecord())
	require.NoError(t, err)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "svc-pod", Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "llminferenceservice", "app.kubernetes.io/instance": serviceName,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name: "storage-initializer",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 0, Message: string(message),
				}},
			}},
		},
	}
}

func TestReconcileGovernanceEvidence_SR2PassesWithVerifiedBaseModel(t *testing.T) {
	svc := baseLLMInferenceService()
	svc.Namespace = "default"
	svc.Spec.Model.URI = "oci://registry.example.com/model@sha256:abc"
	pod := verifiedModelPod(t, svc.Name)

	scheme := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	gr := &evidence.GovernanceReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build(),
	}

	err := gr.Reconcile(context.Background(), svc, nil)
	require.NoError(t, err)

	sr2 := meta.FindStatusCondition(svc.Status.Conditions, "Compliance-SR-2")
	require.NotNil(t, sr2)
	assert.Equal(t, metav1.ConditionTrue, sr2.Status)
	assert.Equal(t, "ProvenanceVerified", sr2.Reason)
}

func TestReconcileGovernanceEvidence_SR2PassesWithVerifiedArtifacts(t *testing.T) {
	gr := &evidence.GovernanceReconciler{}
	llmSvc := baseLLMInferenceService()
	now := metav1.Now()
	activeLoras := []servingv1alpha2.LLMLoraAdapter{
		{
			Status: servingv1alpha2.LLMLoraAdapterStatus{
				StatePlanes: servingv1alpha2.StatePlanes{
					Lifecycle: "active",
					Trust:     "verified",
					Risk:      "normal",
				},
				EvidenceBundle: servingv1alpha2.EvidenceBundle{
					SignatureDigest: "sha256:dummy",
					AttestationURI:  "https://example.invalid/attestation",
					SBOMDigest:      "sha256:sbom",
					LastVerifiedAt:  &now,
				},
			},
		},
	}

	err := gr.Reconcile(context.Background(), llmSvc, activeLoras)
	require.NoError(t, err)

	sr2 := meta.FindStatusCondition(llmSvc.Status.Conditions, "Compliance-SR-2")
	require.NotNil(t, sr2)
	assert.Equal(t, metav1.ConditionTrue, sr2.Status)
	assert.Equal(t, "ProvenanceVerified", sr2.Reason)
}

func TestCleanupResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = servingv1alpha2.AddToScheme(scheme)

	ctx := context.Background()
	llmSvc := baseLLMInferenceService()
	llmSvc.Namespace = "my-ns"
	llmSvc.Name = "my-svc"

	// Mock SPIRE registration ConfigMap
	cmName := security.SPIRERegistrationCMPrefix + llmSvc.Namespace + "-" + llmSvc.Name
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: security.SPIRERegistrationNamespace,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	r := &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		SPIRERegistration: &security.SPIRERegistrationReconciler{
			Client: cl,
			Scheme: scheme,
		},
	}

	// 1. Verify CM exists before cleanup
	var foundCM corev1.ConfigMap
	err := cl.Get(ctx, k8stypes.NamespacedName{Name: cmName, Namespace: security.SPIRERegistrationNamespace}, &foundCM)
	require.NoError(t, err)

	// 2. Run cleanup
	err = r.cleanupResources(ctx, llmSvc)
	require.NoError(t, err)

	// 3. Verify CM is deleted
	err = cl.Get(ctx, k8stypes.NamespacedName{Name: cmName, Namespace: security.SPIRERegistrationNamespace}, &foundCM)
	assert.True(t, apierrors.IsNotFound(err), "ConfigMap should have been deleted by cleanupResources")
}
