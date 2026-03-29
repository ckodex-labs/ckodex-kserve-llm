/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ChunkedPipeliner --------------------------------------------------------

func TestChunkedPipeliner_GetPipelinedEndpoint_NotFound(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	c := NewChunkedPipeliner(pool)
	_, _, ok := c.GetPipelinedEndpoint("session-123")
	assert.False(t, ok)
}

func TestChunkedPipeliner_PushChunk_CreatesSession(t *testing.T) {
	// PushChunk calls FastestEndpoint(nil) which returns "" for empty pools.
	// The session is still created (with empty endpoint), proving PushChunk works.
	pool := NewConnectionPool(DefaultPoolConfig())
	c := NewChunkedPipeliner(pool)
	err := c.PushChunk(context.Background(), "sess1", "some text chunk")
	require.NoError(t, err)

	c.mu.RLock()
	sess, exists := c.sessions["sess1"]
	c.mu.RUnlock()
	assert.True(t, exists)
	assert.NotNil(t, sess)
	assert.Equal(t, "sess1", sess.SessionID)
}

func TestChunkedPipeliner_PushChunk_IncreasesChunkCount(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	pool.Get("ep:8000")

	c := NewChunkedPipeliner(pool)
	_ = c.PushChunk(context.Background(), "sess2", "chunk1")
	_ = c.PushChunk(context.Background(), "sess2", "chunk2")

	c.mu.RLock()
	sess := c.sessions["sess2"]
	c.mu.RUnlock()

	require.NotNil(t, sess)
	assert.Equal(t, 2, sess.Chunks)
}

func TestChunkedPipeliner_GetPipelinedEndpoint_UpdatesLastActive(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	c := NewChunkedPipeliner(pool)
	_ = c.PushChunk(context.Background(), "sess3", "chunk")

	before := time.Now().Add(-1 * time.Millisecond)
	_, _, ok := c.GetPipelinedEndpoint("sess3")
	assert.True(t, ok)

	c.mu.RLock()
	sess := c.sessions["sess3"]
	c.mu.RUnlock()
	assert.True(t, sess.LastActive.After(before) || sess.LastActive.Equal(before))
}

// ---- EBPFConfig --------------------------------------------------------------

func TestEBPFConfig_ApplyBypass_Disabled_NoOp(t *testing.T) {
	cfg := &EBPFConfig{EnableXDP: false}
	annotations := map[string]string{}
	cfg.ApplyBypass(annotations)
	assert.Empty(t, annotations)
}

func TestEBPFConfig_ApplyBypass_NilAnnotations_NoOp(t *testing.T) {
	cfg := &EBPFConfig{EnableXDP: true, TargetDevice: "eth0"}
	cfg.ApplyBypass(nil) // should not panic
}

func TestEBPFConfig_ApplyBypass_Enabled_SetsAnnotations(t *testing.T) {
	cfg := &EBPFConfig{EnableXDP: true, TargetDevice: "eth0", InferencePorts: []int32{8000}}
	annotations := map[string]string{}
	cfg.ApplyBypass(annotations)
	assert.Equal(t, "xdp-bypass-net", annotations["k8s.v1.cni.cncf.io/networks"])
	assert.Equal(t, "enabled", annotations["ebpf.ckodex.io/xdp-acceleration"])
}

// ---- LoRAPinManager ----------------------------------------------------------

func TestLoRAPinManager_Prefetch_Success(t *testing.T) {
	mgr := NewLoRAPinManager(1024)                  // 1024 MB
	err := mgr.Prefetch("adapter-A", 100*1024*1024) // 100MB
	require.NoError(t, err)
}

func TestLoRAPinManager_Prefetch_AlreadyPinned_NoError(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	require.NoError(t, mgr.Prefetch("adapter-A", 100*1024*1024))
	// Second call — already pinned
	require.NoError(t, mgr.Prefetch("adapter-A", 100*1024*1024))
}

func TestLoRAPinManager_Prefetch_ExceedsCapacity_Error(t *testing.T) {
	mgr := NewLoRAPinManager(1)                      // only 1 MB capacity
	err := mgr.Prefetch("big-adapter", 10*1024*1024) // 10 MB — too big
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient pinned memory")
}

func TestLoRAPinManager_Prefetch_EvictsOldestWhenFull(t *testing.T) {
	mgr := NewLoRAPinManager(200) // 200 MB

	// Pin two 100MB adapters
	require.NoError(t, mgr.Prefetch("adapter-A", 100*1024*1024))
	require.NoError(t, mgr.Prefetch("adapter-B", 100*1024*1024))

	// Now pin a third — should evict one (both have PinCount=0)
	err := mgr.Prefetch("adapter-C", 50*1024*1024)
	require.NoError(t, err)
}

func TestLoRAPinManager_LockAdapter_NotPinned_Error(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	err := mgr.LockAdapter("not-pinned")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pre-pinned")
}

func TestLoRAPinManager_LockAdapter_Pinned_Success(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	require.NoError(t, mgr.Prefetch("adapter-A", 10*1024*1024))
	err := mgr.LockAdapter("adapter-A")
	require.NoError(t, err)

	// Should have PinCount=1
	mgr.mu.Lock()
	pin := mgr.pinned["adapter-A"]
	mgr.mu.Unlock()
	assert.Equal(t, int32(1), pin.PinCount)
}

func TestLoRAPinManager_UnlockAdapter_Decrements(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	require.NoError(t, mgr.Prefetch("adapter-A", 10*1024*1024))
	require.NoError(t, mgr.LockAdapter("adapter-A"))
	require.NoError(t, mgr.LockAdapter("adapter-A"))

	mgr.UnlockAdapter("adapter-A")
	mgr.mu.Lock()
	pin := mgr.pinned["adapter-A"]
	mgr.mu.Unlock()
	assert.Equal(t, int32(1), pin.PinCount)
}

func TestLoRAPinManager_UnlockAdapter_NotPinned_NoOp(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	// Should not panic
	mgr.UnlockAdapter("nonexistent")
}

func TestLoRAPinManager_UnlockAdapter_AtZero_StaysZero(t *testing.T) {
	mgr := NewLoRAPinManager(1024)
	require.NoError(t, mgr.Prefetch("adapter-A", 10*1024*1024))
	// PinCount is already 0 — don't go negative
	mgr.UnlockAdapter("adapter-A")

	mgr.mu.Lock()
	pin := mgr.pinned["adapter-A"]
	mgr.mu.Unlock()
	assert.Equal(t, int32(0), pin.PinCount)
}

func TestLoRAPinManager_Prefetch_LockedAdapter_CannotEvict_Error(t *testing.T) {
	mgr := NewLoRAPinManager(200) // 200 MB

	// Pin one adapter and lock it (PinCount > 0 → cannot evict)
	require.NoError(t, mgr.Prefetch("locked", 150*1024*1024))
	require.NoError(t, mgr.LockAdapter("locked"))

	// Try to fit another large adapter — cannot evict locked one
	err := mgr.Prefetch("new-adapter", 100*1024*1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient pinned memory")
}
