package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

func TestLLMLoraAdapterCoverageVerificationRecordReadBranches(t *testing.T) {
	s := buildLoraScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	record, err := readJobVerificationRecord(context.Background(), cl, "default", "job")
	require.NoError(t, err)
	assert.Nil(t, record)

	ignored := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: "default", Labels: map[string]string{"job-name": "job"}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "sidecar", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: "ignored"}}}}}}
	cl = fake.NewClientBuilder().WithScheme(s).WithObjects(ignored).Build()
	record, err = readJobVerificationRecord(context.Background(), cl, "default", "job")
	require.NoError(t, err)
	assert.Nil(t, record)

	malformed := ignored.DeepCopy()
	malformed.Name = "malformed"
	malformed.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "warmup", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: "not-json"}}}}
	cl = fake.NewClientBuilder().WithScheme(s).WithObjects(malformed).Build()
	_, err = readJobVerificationRecord(context.Background(), cl, "default", "job")
	assert.Error(t, err)

	blank := ignored.DeepCopy()
	blank.Name = "blank"
	blank.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "warmup", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: "   "}}}}
	cl = fake.NewClientBuilder().WithScheme(s).WithObjects(blank).Build()
	record, err = readJobVerificationRecord(context.Background(), cl, "default", "job")
	require.NoError(t, err)
	assert.Nil(t, record)
}

func TestLLMLoraAdapterCoverageEvidenceEarlyExitAndTimestampFallback(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("evidence-lora", "default", "svc")
	cache := newLoraCache(lora)
	r := &LLMLoraAdapterReconciler{Client: fake.NewClientBuilder().WithScheme(s).Build()}
	updated, err := r.hydrateVerificationEvidence(context.Background(), lora, cache)
	require.NoError(t, err)
	assert.False(t, updated)

	lora.Spec.Model.URI = "oci://registry.example/lora@sha256:abc"
	cache.Status.NodeStatuses = []servingv1alpha2.NodeCacheStatus{{NodeName: "node", Phase: "Pending", ModelURIHash: "hash"}}
	updated, err = r.hydrateVerificationEvidence(context.Background(), lora, cache)
	require.NoError(t, err)
	assert.False(t, updated)

	payload, err := json.Marshal(provenance.RuntimeVerificationRecord{Subject: lora.Spec.Model.URI, Scheme: "oci", SignatureVerified: true, AttestationVerified: true, SBOMVerified: true, SignatureDigest: "sha256:sig", AttestationURI: "oci://attestation", SBOMDigest: "sha256:sbom", VerifiedAt: "invalid"})
	require.NoError(t, err)
	cache.Status.NodeStatuses[0].Phase = "Ready"
	cache.Status.NodeStatuses[0].ModelURIHash = ModelURIHash(lora.Spec.Model.URI)
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte("node")))[:8]
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "warmup", Namespace: "default", Labels: map[string]string{"job-name": warmupJobPrefix + "-" + cache.Status.NodeStatuses[0].ModelURIHash + "-" + nodeHash}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "warmup", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: string(payload)}}}}}}
	r.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	updated, err = r.hydrateVerificationEvidence(context.Background(), lora, cache)
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, "verified", lora.Status.StatePlanes.Trust)
}

func TestLLMLoraAdapterCoverageEvidenceMissingAndUnverifiedRecords(t *testing.T) {
	s := buildLoraScheme(t)
	lora := testLora("evidence-lora", "default", "svc")
	lora.Spec.Model.URI = "oci://registry.example/lora@sha256:abc"
	cache := newLoraCache(lora)
	cache.Status.NodeStatuses = []servingv1alpha2.NodeCacheStatus{{NodeName: "node", Phase: "Ready", ModelURIHash: ModelURIHash(lora.Spec.Model.URI)}}
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &LLMLoraAdapterReconciler{Client: cl}
	updated, err := r.hydrateVerificationEvidence(context.Background(), lora, cache)
	require.NoError(t, err)
	assert.False(t, updated)

	payload, err := json.Marshal(provenance.RuntimeVerificationRecord{Subject: lora.Spec.Model.URI, Scheme: "oci", SignatureVerified: false, AttestationVerified: true, SBOMVerified: true})
	require.NoError(t, err)
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte("node")))[:8]
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unverified", Namespace: "default", Labels: map[string]string{"job-name": warmupJobPrefix + "-" + cache.Status.NodeStatuses[0].ModelURIHash + "-" + nodeHash}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "warmup", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: string(payload)}}}}}}
	r.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
	updated, err = r.hydrateVerificationEvidence(context.Background(), lora, cache)
	require.NoError(t, err)
	assert.False(t, updated)
}
