package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
	"github.com/ckodex-labs/kserve-llm-operator/internal/storage"
)

func (r *LLMLoraAdapterReconciler) hydrateVerificationEvidence(ctx context.Context, lora *servingv1alpha2.LLMLoraAdapter, cache *servingv1alpha2.LocalModelCache) (bool, error) {
	if !storage.HasOCIScheme(lora.Spec.Model.URI) || len(cache.Status.NodeStatuses) == 0 {
		return false, nil
	}
	verifiedNodes := 0
	var latestRecord *provenance.RuntimeVerificationRecord
	for _, nodeStatus := range cache.Status.NodeStatuses {
		if nodeStatus.Phase != "Ready" {
			return false, nil
		}
		jobName := fmt.Sprintf("%s-%s-%s", warmupJobPrefix, nodeStatus.ModelURIHash, fmt.Sprintf("%x", sha256.Sum256([]byte(nodeStatus.NodeName)))[:8])
		record, err := readJobVerificationRecord(ctx, r.Client, cacheWorkloadNamespace(cache), jobName)
		if err != nil {
			return false, err
		}
		if record == nil || !record.Verified() {
			return false, nil
		}
		verifiedNodes++
		latestRecord = record
	}
	if latestRecord == nil || verifiedNodes == 0 {
		return false, nil
	}
	verifiedAt, err := time.Parse(time.RFC3339, latestRecord.VerifiedAt)
	if err != nil {
		verifiedAt = time.Now().UTC()
	}
	now := metav1.NewTime(verifiedAt)
	lora.Status.EvidenceBundle.SignatureDigest = latestRecord.SignatureDigest
	lora.Status.EvidenceBundle.AttestationURI = latestRecord.AttestationURI
	lora.Status.EvidenceBundle.SBOMDigest = latestRecord.SBOMDigest
	lora.Status.EvidenceBundle.LastVerifiedAt = &now
	lora.Status.StatePlanes.Trust = "verified"
	return true, nil
}

func readJobVerificationRecord(ctx context.Context, c client.Client, namespace, jobName string) (*provenance.RuntimeVerificationRecord, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName}); err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name != "warmup" || status.State.Terminated == nil || strings.TrimSpace(status.State.Terminated.Message) == "" {
				continue
			}
			record, err := provenance.ParseRuntimeVerificationRecord(status.State.Terminated.Message)
			if err != nil {
				return nil, fmt.Errorf("parse warmup verification record from pod %s: %w", pod.Name, err)
			}
			return record, nil
		}
	}
	return nil, nil
}
