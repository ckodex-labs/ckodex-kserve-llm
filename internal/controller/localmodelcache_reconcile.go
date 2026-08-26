/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LocalModelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lmc, err := r.loadLocalModelCache(ctx, req)
	if err != nil || lmc == nil {
		return ctrl.Result{}, err
	}
	if _, err := r.resolveCacheWorkloadNamespace(ctx, lmc); err != nil {
		return ctrl.Result{}, err
	}
	modelHash := ModelURIHash(lmc.Spec.SourceModelURI)
	targetNodes, err := r.resolveTargetNodes(ctx, lmc)
	if err != nil {
		return ctrl.Result{}, err
	}
	nodeStatuses, readyCount := r.reconcileTargetCaches(ctx, lmc, targetNodes, modelHash)
	finalStatuses := mergeNodeCacheStatuses(lmc.Status.NodeStatuses, nodeStatuses, targetNodes, ctx)
	r.evictAndReport(ctx, lmc, finalStatuses)
	cachedModels, totalSize := r.buildCachedModelsStatus(lmc, finalStatuses)
	if err := updateLocalModelCacheStatus(ctx, r, lmc, finalStatuses, readyCount, cachedModels, totalSize); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: localModelCacheRequeueInterval}, nil
}

func (r *LocalModelCacheReconciler) loadLocalModelCache(ctx context.Context, req ctrl.Request) (*servingv1alpha2.LocalModelCache, error) {
	lmc := &servingv1alpha2.LocalModelCache{}
	if err := r.Get(ctx, req.NamespacedName, lmc); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	log.FromContext(ctx).Info("Reconciling LocalModelCache", "name", req.Name, "namespace", req.Namespace)
	return lmc, nil
}

func (r *LocalModelCacheReconciler) reconcileTargetCaches(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, nodes []string, modelHash string) ([]servingv1alpha2.NodeCacheStatus, int32) {
	now := metav1.Now()
	statuses := make([]servingv1alpha2.NodeCacheStatus, 0, len(nodes))
	var readyCount int32
	for _, nodeName := range nodes {
		status, err := r.reconcileNodeCache(ctx, lmc, nodeName, modelHash, now)
		if err != nil {
			r.reportNodeCacheFailure(ctx, lmc, nodeName, err)
			continue
		}
		statuses = append(statuses, status)
		if status.Phase == "Ready" {
			readyCount++
		}
	}
	return statuses, readyCount
}

func (r *LocalModelCacheReconciler) reportNodeCacheFailure(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, nodeName string, err error) {
	log.FromContext(ctx).Error(err, "Failed to reconcile cache for node", "node", nodeName)
	r.Recorder.Eventf(lmc, corev1.EventTypeWarning, "CacheFailed", "Failed to reconcile cache on node %s: %v", nodeName, err)
}

func isNotFound(err error) bool { return errors.IsNotFound(err) }

func mergeNodeCacheStatuses(previous, current []servingv1alpha2.NodeCacheStatus, targets []string, ctx context.Context) []servingv1alpha2.NodeCacheStatus {
	seen := make(map[string]bool, len(targets))
	for _, node := range targets {
		seen[node] = true
	}
	merged := make([]servingv1alpha2.NodeCacheStatus, 0, len(targets))
	for _, prev := range previous {
		if !seen[prev.NodeName] {
			log.FromContext(ctx).Info("Cleanup: Removing stale node status", "node", prev.NodeName)
			continue
		}
		if status, ok := findNodeCacheStatus(current, prev.NodeName); ok {
			merged = append(merged, status)
		} else {
			merged = append(merged, prev)
		}
	}
	for _, status := range current {
		if _, ok := findNodeCacheStatus(merged, status.NodeName); !ok {
			merged = append(merged, status)
		}
	}
	return merged
}

func findNodeCacheStatus(statuses []servingv1alpha2.NodeCacheStatus, nodeName string) (servingv1alpha2.NodeCacheStatus, bool) {
	for _, status := range statuses {
		if status.NodeName == nodeName {
			return status, true
		}
	}
	return servingv1alpha2.NodeCacheStatus{}, false
}

func (r *LocalModelCacheReconciler) evictAndReport(ctx context.Context, lmc *servingv1alpha2.LocalModelCache, statuses []servingv1alpha2.NodeCacheStatus) {
	if err := r.evictLRU(ctx, lmc, statuses); err != nil {
		log.FromContext(ctx).Error(err, "LRU eviction failed")
		r.Recorder.Event(lmc, corev1.EventTypeWarning, "EvictionFailed", err.Error())
	}
}

func updateLocalModelCacheStatus(ctx context.Context, r *LocalModelCacheReconciler, lmc *servingv1alpha2.LocalModelCache, statuses []servingv1alpha2.NodeCacheStatus, readyCount int32, models []servingv1alpha2.CachedModelStatus, totalSize resource.Quantity) error {
	lmc.Status.NodeStatuses = statuses
	lmc.Status.CachedNodes = readyCount
	lmc.Status.CachedModels = models
	lmc.Status.TotalCacheSize = totalSize.String()
	lmc.Status.AvailableSpace = availableCacheSpace(lmc, totalSize)
	if err := r.Status().Update(ctx, lmc); err != nil {
		return fmt.Errorf("updating LocalModelCache status: %w", err)
	}
	return nil
}

func availableCacheSpace(lmc *servingv1alpha2.LocalModelCache, total resource.Quantity) string {
	maxQ, ok := lmc.Spec.MaxCacheSizeQuantity()
	if !ok {
		return ""
	}
	available := maxQ.DeepCopy()
	available.Sub(total)
	if available.Cmp(resource.MustParse("0")) < 0 {
		available = resource.MustParse("0")
	}
	return available.String()
}
