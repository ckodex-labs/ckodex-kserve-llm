/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

func (r *LocalModelCacheReconciler) reconcileNodeCache(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, nodeName, modelHash string, now metav1.Time) (servingv1alpha2.NodeCacheStatus, error) {
	pvcName := PVCNameForNode(modelHash, nodeName)
	status := pendingNodeCacheStatus(nodeName, pvcName, modelHash)
	namespace, err := r.resolveCacheWorkloadNamespace(ctx, lmc)
	if err != nil {
		return status, err
	}
	jobName := warmupJobName(modelHash, nodeName)
	if created, err := r.ensureCachePVC(ctx, lmc, &status, pvcName, namespace, nodeName, modelHash, jobName, now); err != nil || created {
		return status, err
	}
	job, err := r.ensureWarmupJob(ctx, lmc, &status, jobName, pvcName, namespace, nodeName, now)
	if err != nil {
		return status, err
	}
	if job != nil {
		r.updateNodeCacheFromJob(ctx, lmc, &status, job, nodeName, jobName, now)
	}
	return status, nil
}

func pendingNodeCacheStatus(nodeName, pvcName, modelHash string) servingv1alpha2.NodeCacheStatus {
	return servingv1alpha2.NodeCacheStatus{NodeName: nodeName, PVCName: pvcName, Phase: "Pending", ModelURIHash: modelHash}
}

func warmupJobName(modelHash, nodeName string) string {
	nodeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))[:8]
	return fmt.Sprintf("%s-%s-%s", warmupJobPrefix, modelHash, nodeHash)
}

func (r *LocalModelCacheReconciler) ensureCachePVC(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, status *servingv1alpha2.NodeCacheStatus, pvcName, namespace, nodeName, modelHash, jobName string, now metav1.Time) (bool, error) {
	reader := r.cacheReader()
	pvc := &corev1.PersistentVolumeClaim{}
	err := reader.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: namespace}, pvc)
	if err == nil {
		return false, nil
	}
	if !errors.IsNotFound(err) {
		return false, fmt.Errorf("getting PVC %s: %w", pvcName, err)
	}
	orphanJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace}}
	if err := r.Delete(ctx, orphanJob, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !errors.IsNotFound(err) {
		log.FromContext(ctx).Error(err, "Failed to delete orphan warm-up Job", "job", jobName, "namespace", namespace, "node", nodeName, "modelHash", modelHash)
	}
	desired, err := r.buildCachePVC(lmc, pvcName, namespace, nodeName, modelHash)
	if err != nil {
		return false, fmt.Errorf("building PVC %s: %w", pvcName, err)
	}
	log.FromContext(ctx).Info("Creating new cache PVC", "node", nodeName, "pvc", pvcName)
	if err := ctrl.SetControllerReference(lmc, desired, r.Scheme); err != nil {
		return false, fmt.Errorf("setting owner ref on PVC: %w", err)
	}
	if err := r.Create(ctx, desired); err != nil {
		return false, fmt.Errorf("creating PVC %s: %w", pvcName, err)
	}
	status.LastTransitionTime = &now
	return true, nil
}

func (r *LocalModelCacheReconciler) ensureWarmupJob(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, status *servingv1alpha2.NodeCacheStatus, jobName, pvcName, namespace, nodeName string, now metav1.Time) (*batchv1.Job, error) {
	reader := r.cacheReader()
	job := &batchv1.Job{}
	err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: jobName}, job)
	if err == nil {
		if job.DeletionTimestamp != nil {
			log.FromContext(ctx).Info("Waiting for old Job deletion to complete", "job", jobName)
			return nil, nil
		}
		return job, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("getting Job %s: %w", jobName, err)
	}
	job = r.buildWarmupJob(lmc, jobName, pvcName, namespace, nodeName)
	if err := ctrl.SetControllerReference(lmc, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("setting owner ref on Job: %w", err)
	}
	if err := r.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("creating warm-up Job %s: %w", jobName, err)
	}
	status.Phase = "Downloading"
	status.LastTransitionTime = &now
	r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "WarmupStarted", "Started warm-up Job %s on node %s", jobName, nodeName)
	return nil, nil
}

func (r *LocalModelCacheReconciler) updateNodeCacheFromJob(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, status *servingv1alpha2.NodeCacheStatus, job *batchv1.Job, nodeName, jobName string, now metav1.Time) {
	status.Phase = jobPhase(job)
	if status.Phase == "Ready" {
		r.markNodeCacheReady(lmc, status, job, nodeName, now)
	}
	if status.Phase == "Failed" {
		r.selfHealFailedJob(ctx, lmc, status, job, nodeName, jobName)
	}
	status.LastTransitionTime = &now
}

func (r *LocalModelCacheReconciler) markNodeCacheReady(lmc *servingv1alpha2.LocalModelCache, status *servingv1alpha2.NodeCacheStatus, job *batchv1.Job, nodeName string, now metav1.Time) {
	if job.Status.CompletionTime != nil && job.Status.StartTime != nil {
		duration := job.Status.CompletionTime.Sub(job.Status.StartTime.Time).Seconds()
		observability.LMCDownloadDuration.WithLabelValues(lmc.Spec.SourceModelURI, nodeName).Observe(duration)
		observability.LMCWarmingAttempts.WithLabelValues(lmc.Spec.SourceModelURI, nodeName, "success").Inc()
	}
	status.LastUsed = previousReadyUse(lmc, nodeName, now)
	modelSize, err := lmc.Spec.ModelSizeQuantity()
	if err != nil {
		status.Phase = "Failed"
		return
	}
	status.SizeBytes = modelSize.Value()
	observability.LMCCacheSize.WithLabelValues(lmc.Spec.SourceModelURI, nodeName).Set(float64(status.SizeBytes))
}

func previousReadyUse(lmc *servingv1alpha2.LocalModelCache, nodeName string, now metav1.Time) *metav1.Time {
	for _, previous := range lmc.Status.NodeStatuses {
		if previous.NodeName == nodeName && previous.Phase == "Ready" && previous.LastUsed != nil {
			return previous.LastUsed
		}
	}
	return &now
}

func (r *LocalModelCacheReconciler) selfHealFailedJob(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, status *servingv1alpha2.NodeCacheStatus, job *batchv1.Job, nodeName, jobName string) {
	observability.LMCWarmingAttempts.WithLabelValues(lmc.Spec.SourceModelURI, nodeName, "failed").Inc()
	failedTime := failedJobTime(job)
	if failedTime == nil || time.Since(failedTime.Time) <= 5*time.Minute {
		return
	}
	log.FromContext(ctx).Info("Self-Healing: Deleting failed Job for re-warm", "job", jobName, "node", nodeName)
	r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "CacheSelfHealing", "Deleting failed Job %s for re-warm", jobName)
	if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
		log.FromContext(ctx).Error(err, "Failed to delete failed warm-up Job", "job", jobName, "namespace", lmc.Namespace, "node", nodeName, "cache", lmc.Name)
	}
	status.Phase = "Pending"
}

func failedJobTime(job *batchv1.Job) *metav1.Time {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return &condition.LastTransitionTime
		}
	}
	return nil
}

func jobPhase(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return "Ready"
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return "Failed"
		}
	}
	if job.Status.Active > 0 {
		return "Downloading"
	}
	return "Pending"
}
