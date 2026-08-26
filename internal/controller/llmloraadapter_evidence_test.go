/* Copyright 2026 CKodex Authors. Licensed under the Apache License, Version 2.0. */
package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLLMLoraAdapter_HydratesVerifiedEvidenceFromWarmupPod(t *testing.T) {
	s := buildLoraScheme(t)
	modelURI := "oci://registry.example.com/lora@sha256:abc"
	modelHash := ModelURIHash(modelURI)
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte("node-a")))[:8]
	payload, err := json.Marshal(provenance.RuntimeVerificationRecord{Subject: modelURI, Scheme: "oci", SignatureVerified: true, AttestationVerified: true, SBOMVerified: true, SignatureDigest: "sha256:abc", AttestationURI: modelURI + "#attestation:slsaprovenance1", SBOMDigest: "sha256:def", VerifiedAt: "2026-05-11T12:00:00Z"})
	require.NoError(t, err)
	lora := testLora("verified-lora", "default", "my-llm")
	lora.Spec.Model.URI, lora.Spec.Model.Name = modelURI, "verified"
	lora.Status.StatePlanes.Lifecycle = "active"
	cache := newLoraCache(lora)
	cache.Status = servingv1alpha2.LocalModelCacheStatus{Conditions: []metav1.Condition{{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Downloaded", LastTransitionTime: metav1.Now()}}, NodeStatuses: []servingv1alpha2.NodeCacheStatus{{NodeName: "node-a", Phase: "Ready", ModelURIHash: modelHash}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "warmup-pod", Namespace: "default", Labels: map[string]string{"job-name": "lmc-warmup-" + modelHash + "-" + nodeHash}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "warmup", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Message: string(payload)}}}}}}
	target := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(lora, cache, target, pod).WithStatusSubresource(lora).Build()
	r := &LLMLoraAdapterReconciler{Client: cl, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "verified-lora", Namespace: "default"}})
	require.NoError(t, err)
	var updated servingv1alpha2.LLMLoraAdapter
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{Name: "verified-lora", Namespace: "default"}, &updated))
	assert.Equal(t, "sha256:abc", updated.Status.EvidenceBundle.SignatureDigest)
	assert.Equal(t, "sha256:def", updated.Status.EvidenceBundle.SBOMDigest)
	assert.Equal(t, "verified", updated.Status.StatePlanes.Trust)
	require.NotNil(t, updated.Status.EvidenceBundle.LastVerifiedAt)
}
