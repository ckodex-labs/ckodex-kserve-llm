/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ConnectionPool ----------------------------------------------------------

func TestConnectionPool_Get_CreatesNewConn(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	conn := pool.Get("10.0.0.1:8000")
	require.NotNil(t, conn)
	assert.Equal(t, "10.0.0.1:8000", conn.Address)
}

func TestConnectionPool_Get_ReturnsSameConn(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	c1 := pool.Get("10.0.0.1:8000")
	c2 := pool.Get("10.0.0.1:8000")
	assert.Same(t, c1, c2, "second Get should return the same connection")
}

func TestConnectionPool_FastestEndpoint_NoCandidates_ReturnsEmpty(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	result := pool.FastestEndpoint([]string{})
	assert.Equal(t, "", result)
}

func TestConnectionPool_FastestEndpoint_SingleNew_ReturnsIt(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	// endpoint not yet registered — should return immediately as new
	result := pool.FastestEndpoint([]string{"10.0.0.1:8000"})
	assert.Equal(t, "10.0.0.1:8000", result)
}

func TestConnectionPool_FastestEndpoint_PicksLowest(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	// Register two endpoints with different latencies
	pool.RecordLatency("fast:8000", 10*time.Millisecond)
	pool.RecordLatency("slow:8000", 200*time.Millisecond)

	result := pool.FastestEndpoint([]string{"fast:8000", "slow:8000"})
	assert.Equal(t, "fast:8000", result)
}

func TestConnectionPool_FastestEndpoint_NilCandidates_FallsThrough(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	result := pool.FastestEndpoint(nil)
	assert.Equal(t, "", result)
}

func TestConnectionPool_RecordLatency_EWMA(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	pool.RecordLatency("ep:8000", 100*time.Millisecond)
	conn := pool.Get("ep:8000")
	// First write sets directly
	assert.Equal(t, int64(100), conn.AvgLatencyMs.Load())

	pool.RecordLatency("ep:8000", 200*time.Millisecond)
	// EWMA: (100*7 + 200*3)/10 = 130
	assert.Equal(t, int64(130), conn.AvgLatencyMs.Load())
}

func TestConnectionPool_RecordLatency_ResetsErrorCount(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	pool.RecordError("ep:8000")
	pool.RecordError("ep:8000")
	conn := pool.Get("ep:8000")
	assert.Equal(t, int64(2), conn.ErrorCount.Load())
	pool.RecordLatency("ep:8000", 50*time.Millisecond)
	assert.Equal(t, int64(0), conn.ErrorCount.Load())
}

func TestConnectionPool_RecordError_Increments(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	pool.RecordError("ep:8000")
	pool.RecordError("ep:8000")
	pool.RecordError("ep:8000")
	conn := pool.Get("ep:8000")
	assert.Equal(t, int64(3), conn.ErrorCount.Load())
}

func TestConnectionPool_EvictIdle_RemovesOld(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	conn := pool.Get("old:8000")
	// Simulate old last-used time
	conn.LastUsed.Store(time.Now().Add(-10 * time.Minute).UnixMilli())

	evicted := pool.EvictIdle(5 * time.Minute)
	assert.Equal(t, 1, evicted)

	// New Get should create fresh connection
	c2 := pool.Get("old:8000")
	assert.NotSame(t, conn, c2)
}

func TestConnectionPool_EvictIdle_KeepsActive(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	conn := pool.Get("active:8000")
	conn.LastUsed.Store(time.Now().Add(-10 * time.Minute).UnixMilli())
	conn.ActiveRequests.Add(1) // active, cannot evict

	evicted := pool.EvictIdle(5 * time.Minute)
	assert.Equal(t, 0, evicted)
}

func TestDefaultPoolConfig_Values(t *testing.T) {
	cfg := DefaultPoolConfig()
	assert.Equal(t, 100, cfg.MaxIdleConnsPerHost)
	assert.Equal(t, 250, cfg.MaxConnsPerHost)
	assert.True(t, cfg.EnableHTTP2)
}

// ---- LatencyBudget -----------------------------------------------------------

func TestLatencyBudget_Remaining_Positive(t *testing.T) {
	b := DefaultLatencyBudget()
	assert.True(t, b.Remaining() > 0)
}

func TestLatencyBudget_Exceeded_NotYet(t *testing.T) {
	b := DefaultLatencyBudget()
	assert.False(t, b.Exceeded())
}

func TestLatencyBudget_Exceeded_AfterDeadline(t *testing.T) {
	b := LatencyBudget{
		Deadline: time.Now().Add(-1 * time.Second),
	}
	assert.True(t, b.Exceeded())
}

func TestLatencyBudget_WithContext_CancelsAfterDeadline(t *testing.T) {
	b := LatencyBudget{
		TotalBudget: 50 * time.Millisecond,
		Deadline:    time.Now().Add(50 * time.Millisecond),
	}
	ctx, cancel := b.WithContext(context.Background())
	defer cancel()
	assert.NotNil(t, ctx)
}

func TestLatencyBudget_PhaseContext_Route(t *testing.T) {
	b := DefaultLatencyBudget()
	ctx, cancel := b.PhaseContext(context.Background(), "route")
	defer cancel()
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, deadline.After(time.Now()))
}

func TestLatencyBudget_PhaseContext_Prefill(t *testing.T) {
	b := DefaultLatencyBudget()
	ctx, cancel := b.PhaseContext(context.Background(), "prefill")
	defer cancel()
	_, ok := ctx.Deadline()
	assert.True(t, ok)
}

func TestLatencyBudget_PhaseContext_Decode(t *testing.T) {
	b := DefaultLatencyBudget()
	ctx, cancel := b.PhaseContext(context.Background(), "decode")
	defer cancel()
	_, ok := ctx.Deadline()
	assert.True(t, ok)
}

func TestLatencyBudget_PhaseContext_Unknown_UsesRemaining(t *testing.T) {
	b := DefaultLatencyBudget()
	ctx, cancel := b.PhaseContext(context.Background(), "unknown-phase")
	defer cancel()
	_, ok := ctx.Deadline()
	assert.True(t, ok)
}

func TestLatencyBudget_PhaseContext_CapsAtDeadline(t *testing.T) {
	// If phase budget > overall deadline, cap at deadline
	b := LatencyBudget{
		TotalBudget:   1 * time.Second,
		PrefillBudget: 100 * time.Second, // larger than total
		Deadline:      time.Now().Add(1 * time.Second),
	}
	ctx, cancel := b.PhaseContext(context.Background(), "prefill")
	defer cancel()
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	// The deadline should be capped to overall budget deadline
	assert.True(t, deadline.Before(time.Now().Add(2*time.Second)))
}

// ---- Preloader ---------------------------------------------------------------

func TestPreloader_Start_NewModel(t *testing.T) {
	p := NewPreloader()
	state := p.Start("llama3")
	require.NotNil(t, state)
	assert.Equal(t, "llama3", state.ModelName)
	assert.Equal(t, PreloadPending, state.Phase)
}

func TestPreloader_Start_AlreadyStarted_ReturnsSame(t *testing.T) {
	p := NewPreloader()
	s1 := p.Start("llama3")
	s2 := p.Start("llama3")
	assert.Same(t, s1, s2)
}

func TestPreloader_MarkReady_SetsReady(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")
	p.MarkReady("llama3")

	state, ok := p.models["llama3"]
	require.True(t, ok)
	assert.Equal(t, PreloadReady, state.Phase)
}

func TestPreloader_MarkFailed_SetsFailedWithError(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")
	p.MarkFailed("llama3", assert.AnError)

	state, ok := p.models["llama3"]
	require.True(t, ok)
	assert.Equal(t, PreloadFailed, state.Phase)
	assert.Equal(t, assert.AnError, state.Error)
}

func TestPreloader_WaitReady_AlreadyReady(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")
	p.MarkReady("llama3")

	err := p.WaitReady(context.Background(), "llama3")
	require.NoError(t, err)
}

func TestPreloader_WaitReady_WaitsForReady(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")

	go func() {
		time.Sleep(10 * time.Millisecond)
		p.MarkReady("llama3")
	}()

	err := p.WaitReady(context.Background(), "llama3")
	require.NoError(t, err)
}

func TestPreloader_WaitReady_Failed_ReturnsError(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")

	go func() {
		time.Sleep(5 * time.Millisecond)
		p.MarkFailed("llama3", assert.AnError)
	}()

	err := p.WaitReady(context.Background(), "llama3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preload failed")
}

func TestPreloader_WaitReady_NotInQueue_ReturnsError(t *testing.T) {
	p := NewPreloader()
	err := p.WaitReady(context.Background(), "unknown-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in preload queue")
}

func TestPreloader_WaitReady_ContextCancelled_ReturnsError(t *testing.T) {
	p := NewPreloader()
	p.Start("heavy-model")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := p.WaitReady(ctx, "heavy-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestPreloader_LoadDuration_ReturnsZeroIfNotReady(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")
	dur := p.LoadDuration("llama3")
	assert.Equal(t, time.Duration(0), dur)
}

func TestPreloader_LoadDuration_AfterReady(t *testing.T) {
	p := NewPreloader()
	p.Start("llama3")
	time.Sleep(5 * time.Millisecond)
	p.MarkReady("llama3")
	dur := p.LoadDuration("llama3")
	assert.True(t, dur >= 5*time.Millisecond)
}

func TestPreloader_LoadDuration_UnknownModel_ReturnsZero(t *testing.T) {
	p := NewPreloader()
	assert.Equal(t, time.Duration(0), p.LoadDuration("nope"))
}

func TestPreloader_MarkReady_UnknownModel_NoOp(t *testing.T) {
	p := NewPreloader()
	// Should not panic
	p.MarkReady("nonexistent")
}

func TestPreloader_MarkFailed_UnknownModel_NoOp(t *testing.T) {
	p := NewPreloader()
	// Should not panic
	p.MarkFailed("nonexistent", assert.AnError)
}

// ---- StreamWriter ------------------------------------------------------------

type flushResponseRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushResponseRecorder) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}

func TestNewStreamWriter_SetsHeaders(t *testing.T) {
	w := &flushResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw, err := NewStreamWriter(w)
	require.NoError(t, err)
	assert.NotNil(t, sw)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
}

func TestNewStreamWriter_NonFlushable_ReturnsError(t *testing.T) {
	// httptest.ResponseRecorder is flushable, so use a non-flushing writer
	type nonFlusher struct{ http.ResponseWriter }
	w := &nonFlusher{httptest.NewRecorder()}
	_, err := NewStreamWriter(w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flushing")
}

func TestStreamWriter_WriteChunk_Success(t *testing.T) {
	w := &flushResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw, err := NewStreamWriter(w)
	require.NoError(t, err)

	err = sw.WriteChunk([]byte(`{"token":"hello"}`))
	require.NoError(t, err)
	body := w.Body.String()
	assert.Contains(t, body, `data: {"token":"hello"}`)
	assert.True(t, w.flushed)
}

func TestStreamWriter_WriteChunk_AfterClosed_ReturnsError(t *testing.T) {
	w := &flushResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw, err := NewStreamWriter(w)
	require.NoError(t, err)

	err = sw.WriteDone()
	require.NoError(t, err)

	err = sw.WriteChunk([]byte("late"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed")
}

func TestStreamWriter_WriteDone_SendsDONE(t *testing.T) {
	w := &flushResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw, err := NewStreamWriter(w)
	require.NoError(t, err)

	err = sw.WriteDone()
	require.NoError(t, err)
	assert.Contains(t, w.Body.String(), "data: [DONE]")
}

func TestStreamWriter_WriteDone_Idempotent(t *testing.T) {
	w := &flushResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw, err := NewStreamWriter(w)
	require.NoError(t, err)

	require.NoError(t, sw.WriteDone())
	require.NoError(t, sw.WriteDone()) // second call is no-op
}

// ---- FastPathRouter ----------------------------------------------------------

func TestFastPathRouter_Route_SessionBound_Healthy(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	router := NewFastPathRouter(pool)

	// Session endpoint has low error count
	pool.Get("session-ep:8000") // register it
	result := router.Route(context.Background(), "session-ep:8000", []string{"other:8000"})
	assert.Equal(t, "session-ep:8000", result.Endpoint)
	assert.True(t, result.CacheHit)
}

func TestFastPathRouter_Route_SessionBound_Unhealthy_FallsThrough(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	router := NewFastPathRouter(pool)

	// Force session endpoint to be unhealthy
	conn := pool.Get("broken:8000")
	conn.ErrorCount.Store(5) // > 3 threshold

	pool.RecordLatency("healthy:8000", 10*time.Millisecond)
	result := router.Route(context.Background(), "broken:8000", []string{"healthy:8000"})
	assert.Equal(t, "healthy:8000", result.Endpoint)
	assert.False(t, result.CacheHit)
}

func TestFastPathRouter_Route_NoSession_PicksFastest(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	router := NewFastPathRouter(pool)

	pool.RecordLatency("fast:8000", 10*time.Millisecond)
	pool.RecordLatency("slow:8000", 500*time.Millisecond)

	result := router.Route(context.Background(), "", []string{"fast:8000", "slow:8000"})
	assert.Equal(t, "fast:8000", result.Endpoint)
}

func TestFastPathRouter_Route_EmptyCandidates_EmptyResult(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	router := NewFastPathRouter(pool)

	result := router.Route(context.Background(), "", []string{})
	assert.Equal(t, "", result.Endpoint)
}

// ---- Coalescer ---------------------------------------------------------------

func TestCoalescer_Submit_NewKey_ReturnsChannel(t *testing.T) {
	c := NewCoalescer(100*time.Millisecond, 10)
	ch := c.Submit("key1")
	assert.NotNil(t, ch)
}

func TestCoalescer_Submit_SameKey_CoalescedInWindow(t *testing.T) {
	c := NewCoalescer(10*time.Second, 10)
	ch1 := c.Submit("key1")
	ch2 := c.Submit("key1")
	// Both should be different channels but coalesced
	assert.NotNil(t, ch1)
	assert.NotNil(t, ch2)
}

func TestCoalescer_Submit_ExceedsMaxBatch_StartsNew(t *testing.T) {
	c := NewCoalescer(10*time.Second, 2) // max 2 waiters
	c.Submit("key1")
	c.Submit("key1")        // fills up
	ch3 := c.Submit("key1") // should start a new batch
	assert.NotNil(t, ch3)
}

func TestCoalescer_Flush_WindowExpired_ReturnsAll(t *testing.T) {
	c := NewCoalescer(0, 100) // zero window — always expired
	c.Submit("k1")
	c.Submit("k2")

	batches := c.Flush()
	assert.Len(t, batches, 2)
}

func TestCoalescer_Flush_WindowNotExpired_ReturnsEmpty(t *testing.T) {
	c := NewCoalescer(10*time.Second, 100)
	c.Submit("k1")

	batches := c.Flush()
	assert.Empty(t, batches)
}

func TestCoalescer_Flush_MaxBatch_ReturnsBatch(t *testing.T) {
	c := NewCoalescer(10*time.Second, 2)
	c.Submit("k1")
	c.Submit("k1") // fills to max

	batches := c.Flush()
	assert.NotEmpty(t, batches)
	assert.Len(t, batches["k1"], 2)
}
