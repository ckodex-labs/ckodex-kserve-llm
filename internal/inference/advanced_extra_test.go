/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoTCP skips the test if TCP binding is unavailable (sandbox restriction).
func skipIfNoTCP(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP binding unavailable in this environment: %v", err)
	}
	_ = ln.Close()
}

// ---- PrefixCacheWarmer -------------------------------------------------------

func TestPrefixCacheWarmer_Register_NewPrefix(t *testing.T) {
	w := NewPrefixCacheWarmer()
	entry := w.Register("hello world", 3)
	require.NotNil(t, entry)
	assert.Equal(t, "hello world", entry.Text)
	assert.Equal(t, int32(3), entry.TokenCount)
}

func TestPrefixCacheWarmer_Register_Idempotent(t *testing.T) {
	w := NewPrefixCacheWarmer()
	e1 := w.Register("prefix", 5)
	e2 := w.Register("prefix", 5)
	assert.Same(t, e1, e2)
}

func TestPrefixCacheWarmer_LookupEndpoint_NotRegistered_ReturnsNil(t *testing.T) {
	w := NewPrefixCacheWarmer()
	eps := w.LookupEndpoint("unknown prefix")
	assert.Nil(t, eps)
}

func TestPrefixCacheWarmer_LookupEndpoint_NoCachedEndpoints(t *testing.T) {
	w := NewPrefixCacheWarmer()
	w.Register("system prompt", 10)
	eps := w.LookupEndpoint("system prompt")
	assert.Empty(t, eps)
}

func TestPrefixCacheWarmer_MarkCached_ThenLookup(t *testing.T) {
	w := NewPrefixCacheWarmer()
	w.Register("system prompt", 10)
	w.MarkCached("system prompt", "ep1:8000")
	w.MarkCached("system prompt", "ep2:8000")

	eps := w.LookupEndpoint("system prompt")
	assert.Len(t, eps, 2)
	assert.Contains(t, eps, "ep1:8000")
	assert.Contains(t, eps, "ep2:8000")
}

func TestPrefixCacheWarmer_MarkCached_DuplicateEndpoint_Ignored(t *testing.T) {
	w := NewPrefixCacheWarmer()
	w.Register("system prompt", 10)
	w.MarkCached("system prompt", "ep1:8000")
	w.MarkCached("system prompt", "ep1:8000") // duplicate

	eps := w.LookupEndpoint("system prompt")
	assert.Len(t, eps, 1)
}

func TestPrefixCacheWarmer_MarkCached_UnregisteredPrefix_NoOp(t *testing.T) {
	w := NewPrefixCacheWarmer()
	// Should not panic
	w.MarkCached("unknown prefix", "ep1:8000")
}

func TestPrefixCacheWarmer_EvictStale_RemovesOldEntry(t *testing.T) {
	w := NewPrefixCacheWarmer()
	w.Register("stale prefix", 5)
	// LastUsed defaults to zero (never used), HitCount is 0 — should be evicted

	evicted := w.EvictStale(1 * time.Millisecond)
	assert.Equal(t, 1, evicted)
}

func TestPrefixCacheWarmer_EvictStale_KeepsRecentlyUsed(t *testing.T) {
	w := NewPrefixCacheWarmer()
	w.Register("active prefix", 5)
	// Simulate recent lookup
	w.LookupEndpoint("active prefix")

	// With a long maxAge (1 hour), nothing should be evicted since lastUsed is ~now
	evicted := w.EvictStale(1 * time.Hour)
	assert.Equal(t, 0, evicted)
}

// ---- hashPrefix --------------------------------------------------------------

func TestHashPrefix_Deterministic(t *testing.T) {
	h1 := hashPrefix("hello")
	h2 := hashPrefix("hello")
	assert.Equal(t, h1, h2)
}

func TestHashPrefix_DifferentInputs_DifferentHashes(t *testing.T) {
	h1 := hashPrefix("hello")
	h2 := hashPrefix("world")
	assert.NotEqual(t, h1, h2)
}

// ---- SpeculativeDecoder ------------------------------------------------------

func TestSpeculativeDecoder_New_EnabledByDefault(t *testing.T) {
	sd := NewSpeculativeDecoder("small-model", "large-model", 4)
	assert.True(t, sd.Enabled.Load())
	assert.Equal(t, "small-model", sd.DraftModel)
	assert.Equal(t, "large-model", sd.TargetModel)
	assert.Equal(t, 4, sd.LookaheadTokens)
}

func TestSpeculativeDecoder_ShouldSpeculate_Disabled_False(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	sd.Enabled.Store(false)
	assert.False(t, sd.ShouldSpeculate(100))
}

func TestSpeculativeDecoder_ShouldSpeculate_TooFewTokens_False(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	assert.False(t, sd.ShouldSpeculate(10)) // < 32
}

func TestSpeculativeDecoder_ShouldSpeculate_LowAcceptance_False(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	sd.AcceptanceRate.Store(4000) // < 5000
	assert.False(t, sd.ShouldSpeculate(100))
}

func TestSpeculativeDecoder_ShouldSpeculate_NormalCase_True(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	// Default acceptance rate is 8500 > 5000, maxTokens=100 > 32
	assert.True(t, sd.ShouldSpeculate(100))
}

func TestSpeculativeDecoder_RecordAcceptance_ZeroProposed_NoOp(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	before := sd.AcceptanceRate.Load()
	sd.RecordAcceptance(0, 0)
	assert.Equal(t, before, sd.AcceptanceRate.Load())
}

func TestSpeculativeDecoder_RecordAcceptance_Updates(t *testing.T) {
	sd := NewSpeculativeDecoder("small", "large", 4)
	// Initially 8500. Record 100% acceptance: rate=10000
	// Updated = (8500*7 + 10000*3)/10 = (59500 + 30000)/10 = 8950
	sd.RecordAcceptance(4, 4)
	assert.Equal(t, int64(8950), sd.AcceptanceRate.Load())
}

// ---- TTIMetrics / ThroughputTracker / RateTracker ----------------------------

func TestNewTTIMetrics_Initialized(t *testing.T) {
	m := NewTTIMetrics()
	require.NotNil(t, m.TTFT)
	require.NotNil(t, m.E2E)
	require.NotNil(t, m.TokenThroughput)
	require.NotNil(t, m.CacheHitRate)
}

func TestTTIMetrics_RecordRequest_UpdatesCounters(t *testing.T) {
	m := NewTTIMetrics()
	m.RecordRequest(100*time.Millisecond, 500*time.Millisecond, 128, true)
	m.RecordRequest(200*time.Millisecond, 800*time.Millisecond, 256, false)

	assert.Equal(t, int64(2), m.TotalRequests.Load())
}

func TestTTIMetrics_Snapshot_NonNil(t *testing.T) {
	m := NewTTIMetrics()
	snap := m.Snapshot()
	assert.Equal(t, int64(0), snap.TotalRequests)
	assert.Equal(t, int64(0), snap.ActiveRequests)
}

func TestTTIMetrics_Snapshot_AfterRequests(t *testing.T) {
	m := NewTTIMetrics()
	m.RecordRequest(100*time.Millisecond, 500*time.Millisecond, 1000, true)
	snap := m.Snapshot()
	assert.Equal(t, int64(1), snap.TotalRequests)
}

func TestThroughputTracker_RecordTokens_AddsUp(t *testing.T) {
	tr := NewThroughputTracker()
	tr.RecordTokens(100, 0)
	tr.RecordTokens(200, 0)
	// totalTokens should be 300
	assert.Equal(t, int64(300), tr.totalTokens.Load())
}

func TestThroughputTracker_TokensPerSecond_LessThan1s_ReturnsZero(t *testing.T) {
	tr := NewThroughputTracker()
	tr.windowStart.Store(time.Now().UnixMilli())
	tr.RecordTokens(100, 0)
	// Less than 1 second elapsed
	assert.Equal(t, float64(0), tr.TokensPerSecond())
}

func TestThroughputTracker_TokensPerSecond_After1s_Returns(t *testing.T) {
	tr := NewThroughputTracker()
	tr.windowStart.Store(time.Now().Add(-2 * time.Second).UnixMilli())
	tr.RecordTokens(200, 0)
	tps := tr.TokensPerSecond()
	assert.True(t, tps > 0)
	assert.True(t, tps <= 200) // 200 tokens over ~2s ≤ 200 tps
}

func TestRateTracker_Percent_NoData_ReturnsZero(t *testing.T) {
	rt := NewRateTracker()
	assert.Equal(t, float64(0), rt.Percent())
}

func TestRateTracker_Percent_AllHits(t *testing.T) {
	rt := NewRateTracker()
	rt.RecordHit()
	rt.RecordHit()
	assert.Equal(t, float64(100), rt.Percent())
}

func TestRateTracker_Percent_AllMisses(t *testing.T) {
	rt := NewRateTracker()
	rt.RecordMiss()
	rt.RecordMiss()
	assert.Equal(t, float64(0), rt.Percent())
}

func TestRateTracker_Percent_HalfAndHalf(t *testing.T) {
	rt := NewRateTracker()
	rt.RecordHit()
	rt.RecordMiss()
	assert.Equal(t, float64(50), rt.Percent())
}

// ---- WarmConnections + newHealthRequest --------------------------------------

func TestNewHealthRequest_HasCorrectURL(t *testing.T) {
	req, err := newHealthRequest(context.Background(), "10.0.0.1:8080")
	require.NoError(t, err)
	assert.Equal(t, "http://10.0.0.1:8080/v2/health/ready", req.URL.String())
	assert.Equal(t, "GET", req.Method)
}

func TestWarmConnections_ReachableEndpoints(t *testing.T) {
	skipIfNoTCP(t)
	// Create a test server that responds to health checks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/health/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pool := NewConnectionPool(DefaultPoolConfig())
	addr := server.URL[len("http://"):]
	err := pool.WarmConnections(context.Background(), []string{addr})
	require.NoError(t, err)
}

func TestWarmConnections_UnreachableEndpoints_NoError(t *testing.T) {
	// WarmConnections is best-effort — errors are swallowed
	pool := NewConnectionPool(DefaultPoolConfig())
	err := pool.WarmConnections(context.Background(), []string{"localhost:19999"})
	require.NoError(t, err)
}

func TestWarmConnections_EmptyEndpoints_NoError(t *testing.T) {
	pool := NewConnectionPool(DefaultPoolConfig())
	err := pool.WarmConnections(context.Background(), []string{})
	require.NoError(t, err)
}
