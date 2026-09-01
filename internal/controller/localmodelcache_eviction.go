/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

type localModelCacheEntry struct {
	index int
	last  time.Time
	size  int64
}

func (r *LocalModelCacheReconciler) evictLRU(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, statuses []servingv1alpha2.NodeCacheStatus) error {
	maxQ, ok, err := lmc.Spec.MaxCacheSizeQuantity()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	entries := cacheEntries(statuses)
	total := cacheEntriesSize(entries)
	for _, entry := range entries {
		if total.Cmp(maxQ) <= 0 {
			break
		}
		if err := r.evictCacheEntry(ctx, lmc, statuses, entry); err != nil {
			return err
		}
		total.Sub(*resource.NewQuantity(entry.size, resource.BinarySI))
	}
	return nil
}

func cacheEntries(statuses []servingv1alpha2.NodeCacheStatus) []localModelCacheEntry {
	entries := make([]localModelCacheEntry, 0, len(statuses))
	for index, status := range statuses {
		if status.Phase != "Ready" {
			continue
		}
		last := time.Time{}
		if status.LastUsed != nil {
			last = status.LastUsed.Time
		}
		entries = append(entries, localModelCacheEntry{index: index, last: last, size: status.SizeBytes})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].last.Before(entries[j].last) })
	return entries
}

func cacheEntriesSize(entries []localModelCacheEntry) resource.Quantity {
	total := resource.Quantity{}
	for _, entry := range entries {
		total.Add(*resource.NewQuantity(entry.size, resource.BinarySI))
	}
	return total
}

func (r *LocalModelCacheReconciler) evictCacheEntry(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, statuses []servingv1alpha2.NodeCacheStatus, entry localModelCacheEntry) error {
	status := statuses[entry.index]
	log.FromContext(ctx).Info("LRU evicting cache PVC", "pvc", status.PVCName, "node", status.NodeName, "lastUsed", entry.last, "size", entry.size)
	namespace, err := r.resolveCacheWorkloadNamespace(ctx, lmc)
	if err != nil {
		return err
	}
	if err := r.deleteCachePVC(ctx, namespace, status.PVCName); err != nil {
		return err
	}
	if err := r.deleteCacheJob(ctx, namespace, warmupJobName(status.ModelURIHash, status.NodeName)); err != nil {
		return err
	}
	statuses[entry.index].Phase = "Pending"
	r.Recorder.Eventf(lmc, corev1.EventTypeNormal, "CacheEvicted", "LRU evicted PVC %s on node %s (size=%d)", status.PVCName, status.NodeName, entry.size)
	return nil
}

func (r *LocalModelCacheReconciler) deleteCachePVC(ctx context.Context, namespace, name string) error {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pvc); err == nil {
		if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting evicted PVC %s: %w", name, err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("getting PVC %s for deletion: %w", name, err)
	}
	return nil
}

func (r *LocalModelCacheReconciler) deleteCacheJob(ctx context.Context, namespace, name string) error {
	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, job); err == nil {
		propagation := metav1.DeletePropagationBackground
		if err := r.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting evicted Job %s: %w", name, err)
		}
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("getting Job %s for deletion: %w", name, err)
	}
	return nil
}

func cachedModelStatus(lmc *servingv1alpha2.LocalModelCache, nodeNames []string, totalBytes int64, latestUsed *metav1.Time) []servingv1alpha2.CachedModelStatus {
	if len(nodeNames) == 0 {
		return nil
	}
	return []servingv1alpha2.CachedModelStatus{{ModelURI: lmc.Spec.SourceModelURI, NodeNames: nodeNames, SizeBytes: totalBytes, LastUsed: latestUsed, PVCName: PVCNameForNode(ModelURIHash(lmc.Spec.SourceModelURI), nodeNames[0])}}
}

func (r *LocalModelCacheReconciler) buildCachedModelsStatus(lmc *servingv1alpha2.LocalModelCache, statuses []servingv1alpha2.NodeCacheStatus) ([]servingv1alpha2.CachedModelStatus, resource.Quantity) {
	total := resource.Quantity{}
	nodeNames := []string{}
	var latestUsed *metav1.Time
	var totalBytes int64
	for _, status := range statuses {
		if status.Phase != "Ready" {
			continue
		}
		nodeNames = append(nodeNames, status.NodeName)
		totalBytes += status.SizeBytes
		total.Add(*resource.NewQuantity(status.SizeBytes, resource.BinarySI))
		if status.LastUsed != nil && (latestUsed == nil || status.LastUsed.After(latestUsed.Time)) {
			latestUsed = status.LastUsed
		}
	}
	return cachedModelStatus(lmc, nodeNames, totalBytes, latestUsed), total
}
