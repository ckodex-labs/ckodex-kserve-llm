/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Preloader handles background model loading to eliminate cold-start latency.
type Preloader struct {
	mu     sync.RWMutex
	models map[string]*PreloadState
}

// PreloadState tracks a single model's loading state.
type PreloadState struct {
	ModelName string
	Phase     PreloadPhase
	StartTime time.Time
	ReadyTime time.Time
	Error     error
	ReadyCh   chan struct{} // Closed when model is ready
}

// PreloadPhase represents model loading phases.
type PreloadPhase string

const (
	PreloadPending     PreloadPhase = "Pending"
	PreloadDownloading PreloadPhase = "Downloading"
	PreloadLoading     PreloadPhase = "Loading"
	PreloadWarming     PreloadPhase = "Warming"
	PreloadReady       PreloadPhase = "Ready"
	PreloadFailed      PreloadPhase = "Failed"
)

// NewPreloader creates a preloader.
func NewPreloader() *Preloader {
	return &Preloader{
		models: make(map[string]*PreloadState),
	}
}

// Start begins preloading a model in the background.
func (p *Preloader) Start(modelName string) *PreloadState {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		return state
	}

	state := &PreloadState{
		ModelName: modelName,
		Phase:     PreloadPending,
		StartTime: time.Now(),
		ReadyCh:   make(chan struct{}),
	}
	p.models[modelName] = state
	return state
}

// MarkReady signals that a model is loaded and warmed up.
func (p *Preloader) MarkReady(modelName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		state.Phase = PreloadReady
		state.ReadyTime = time.Now()
		close(state.ReadyCh)
	}
}

// MarkFailed records a model load failure.
func (p *Preloader) MarkFailed(modelName string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, ok := p.models[modelName]; ok {
		state.Phase = PreloadFailed
		state.Error = err
		close(state.ReadyCh)
	}
}

// WaitReady blocks until the model is ready or the context expires.
func (p *Preloader) WaitReady(ctx context.Context, modelName string) error {
	p.mu.RLock()
	state, ok := p.models[modelName]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("model %s not in preload queue", modelName)
	}

	select {
	case <-state.ReadyCh:
		// Channel close happens-before this receive, so fields are safe to read.
		if state.Error != nil {
			return fmt.Errorf("model %s preload failed: %w", modelName, state.Error)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("model %s preload timed out: %w", modelName, ctx.Err())
	}
}

// LoadDuration returns how long the model took to load.
func (p *Preloader) LoadDuration(modelName string) time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if state, ok := p.models[modelName]; ok && state.Phase == PreloadReady {
		return state.ReadyTime.Sub(state.StartTime)
	}
	return 0
}
