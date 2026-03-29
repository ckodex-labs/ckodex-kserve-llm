/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"fmt"
	"sync"
)

// StorageClient defines the interface for pulling model artifacts from various hubs.
type StorageClient interface {
	// Pull downloads the artifact from the given URI to the destination path.
	Pull(ctx context.Context, uri string, destPath string) error

	// Schemes returns the URI schemes supported by this client (e.g., "hf", "oci").
	Schemes() []string
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]StorageClient)
)

// RegisterClient registers a storage client for its supported schemes.
func RegisterClient(client StorageClient) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, scheme := range client.Schemes() {
		registry[scheme] = client
	}
}

// GetClient returns the appropriate StorageClient for the given scheme.
func GetClient(scheme string) (StorageClient, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	client, ok := registry[scheme]
	if !ok {
		return nil, fmt.Errorf("no storage client registered for scheme: %s", scheme)
	}
	return client, nil
}
