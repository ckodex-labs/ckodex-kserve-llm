/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package deployment

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const hardwareCacheTTL = 5 * time.Minute

// HardwareCache caches the detected cluster hardware type.
// Avoids listing all nodes on every reconcile iteration — at scale
// a node List is an unbounded API call.
type HardwareCache struct {
	mu        sync.RWMutex
	hardware  HardwareType
	cacheTime time.Time
}

// Get returns the detected hardware type, refreshing from the API reader when
// the cache has expired. reader is preferred over c for bypass-cache semantics.
func (h *HardwareCache) Get(ctx context.Context, c client.Client, reader client.Reader) HardwareType {
	h.mu.RLock()
	if time.Since(h.cacheTime) < hardwareCacheTTL {
		hw := h.hardware
		h.mu.RUnlock()
		return hw
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring write lock.
	if time.Since(h.cacheTime) < hardwareCacheTTL {
		return h.hardware
	}

	r := reader
	if r == nil {
		r = c
	}
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList); err != nil {
		log.FromContext(ctx).Error(err, "unable to list nodes for hardware detection, using cached value")
		return h.hardware
	}

	h.hardware = DetectHardware(nodeList.Items)
	h.cacheTime = time.Now()
	return h.hardware
}

// PtrToHostPath returns a pointer to a HostPathType value.
func PtrToHostPath(hp corev1.HostPathType) *corev1.HostPathType {
	return &hp
}
