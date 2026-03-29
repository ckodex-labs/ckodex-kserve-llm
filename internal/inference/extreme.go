/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// --- Cache Backend Interface ---

// CacheBackend is the storage contract for the SemanticCache.
// Two implementations exist:
//   - InMemoryCache  — zero-dependency, suitable for single-replica and test environments.
//   - RedisCache     — distributed, TTL-aware, suitable for multi-replica production.
//
// The factory NewSemanticCache selects the implementation based on OperatorConfig.
type CacheBackend interface {
	// Get returns the cached response for the given SHA-256 hex key.
	// Returns (response, true) on a hit, ("", false) on a miss.
	Get(ctx context.Context, key string) (string, bool)

	// Set stores response under key with the given TTL.
	// A TTL of 0 means "no expiry" (in-memory) or the backend default (Redis).
	Set(ctx context.Context, key string, response string, ttl time.Duration)

	// Close releases any resources held by the backend (e.g., Redis connection pool).
	// Idempotent. Must not panic on double-close.
	Close() error
}

// --- In-Memory Implementation ---

// inMemoryCache is a thread-safe in-memory cache for single-replica / test use.
// It does not evict entries — callers accept unbounded growth in exchange for
// zero external dependencies.
type inMemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*memCacheEntry
}

type memCacheEntry struct {
	response   string
	hitCount   int64
	lastAccess time.Time
	expiresAt  time.Time // zero means no expiry
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{entries: make(map[string]*memCacheEntry)}
}

func (c *inMemoryCache) Get(_ context.Context, key string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return "", false
	}
	c.mu.Lock()
	entry.hitCount++
	entry.lastAccess = time.Now()
	c.mu.Unlock()
	return entry.response, true
}

func (c *inMemoryCache) Set(_ context.Context, key, response string, ttl time.Duration) {
	entry := &memCacheEntry{
		response:   response,
		hitCount:   1,
		lastAccess: time.Now(),
	}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

func (c *inMemoryCache) Close() error { return nil }

// --- Redis / Valkey Implementation ---

// redisCache stores inference responses in Redis/Valkey with TTL-based eviction.
// Backed by go-redis v9 which supports Redis 6+ and Valkey natively.
type redisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// newRedisCache dials the given addr and returns a ready-to-use redisCache.
// addr format: "host:port" (e.g., "valkey:6379").
// A Ping is attempted to surface misconfigurations at startup rather than at
// first cache miss in the hot path.
func newRedisCache(ctx context.Context, addr string, ttl time.Duration) (*redisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		// Pool sized to match typical operator concurrency (20 reconcile goroutines).
		PoolSize:     20,
		MinIdleConns: 5,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}
	return &redisCache{client: client, ttl: ttl}, nil
}

func (r *redisCache) Get(ctx context.Context, key string) (string, bool) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		// redis.Nil is a normal miss; other errors are transient — treat both as miss
		// to preserve availability (cache should never block inference).
		return "", false
	}
	return val, true
}

func (r *redisCache) Set(ctx context.Context, key, response string, ttl time.Duration) {
	effective := ttl
	if effective == 0 {
		effective = r.ttl
	}
	// Fire-and-forget: a SET failure degrades to a miss on the next request.
	// We intentionally do not surface this error to callers.
	_ = r.client.Set(ctx, key, response, effective).Err()
}

func (r *redisCache) Close() error {
	return r.client.Close()
}

// --- SemanticCache (public facade) ---

// SemanticCache provides exact-match caching of inference responses keyed by
// SHA-256(prompt). A hit returns the precomputed response in microseconds,
// consuming zero GPU cycles — critical for TTFT on repeated standard prompts
// (e.g., RAG pipelines with near-identical user queries).
//
// The underlying backend is selected by NewSemanticCache based on whether a
// Redis address is configured.
type SemanticCache struct {
	backend CacheBackend
	ttl     time.Duration
}

// NewSemanticCache constructs a SemanticCache backed by Redis when addr is non-empty,
// falling back to an in-memory cache otherwise.
func NewSemanticCache(ctx context.Context, addr string, ttl time.Duration) (*SemanticCache, error) {
	if addr == "" {
		return &SemanticCache{backend: newInMemoryCache(), ttl: ttl}, nil
	}
	rb, err := newRedisCache(ctx, addr, ttl)
	if err != nil {
		return nil, fmt.Errorf("semantic cache: %w", err)
	}
	return &SemanticCache{backend: rb, ttl: ttl}, nil
}

// GetExact returns a cached response if the prompt hash matches exactly.
func (s *SemanticCache) GetExact(ctx context.Context, prompt string) (string, bool) {
	return s.backend.Get(ctx, hashPrompt(prompt))
}

// StoreExact saves a response keyed by the prompt hash.
func (s *SemanticCache) StoreExact(ctx context.Context, prompt, response string) {
	s.backend.Set(ctx, hashPrompt(prompt), response, s.ttl)
}

// Close releases backend resources. Must be called on operator shutdown.
func (s *SemanticCache) Close() error {
	return s.backend.Close()
}

// mustInMemoryCache returns an in-memory SemanticCache with a 1-hour TTL.
// Intended for test and dev use where no Redis address is configured.
func mustInMemoryCache() *SemanticCache {
	return &SemanticCache{backend: newInMemoryCache(), ttl: time.Hour}
}

func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(h[:])
}

// --- Anticipatory Prefetching ---

// Intent defines a partial user prompt received before they hit "send".
type Intent struct {
	UserID        string
	SessionID     string
	PartialPrompt string
	Confidence    float64
}

// AnticipatoryPrefetcher warms up connections and pre-allocates KV cache slots
// while the user is still typing. This shifts routing and connection overhead
// out of the critical path (the time between "click" and "first token").
type AnticipatoryPrefetcher struct {
	pool      *ConnectionPool
	router    *FastPathRouter
	warmPaths sync.Map // map[sessionID]string (endpoint address)
}

// NewAnticipatoryPrefetcher creates an anticipatory pipeline.
func NewAnticipatoryPrefetcher(pool *ConnectionPool, router *FastPathRouter) *AnticipatoryPrefetcher {
	return &AnticipatoryPrefetcher{
		pool:   pool,
		router: router,
	}
}

// HandleIntent processes a keystroke/typing event.
// If the user seems committed to sending a message (high confidence),
// it pre-selects an endpoint and warms the TCP+TLS connection.
func (a *AnticipatoryPrefetcher) HandleIntent(ctx context.Context, intent Intent, candidates []string) {
	// Only prefetch if we are reasonably sure they will submit
	if intent.Confidence < 0.7 {
		return
	}

	// 1. Pre-route: Find the best endpoint now, before the full request arrives
	result := a.router.Route(ctx, intent.SessionID, candidates)
	if result.Endpoint == "" {
		return
	}

	// 2. Warm the connection
	conn := a.pool.Get(result.Endpoint)

	// Lightweight ping to establish TLS outside the critical path
	go func() {
		req, err := newHealthRequest(ctx, result.Endpoint)
		if err == nil {
			resp, err := conn.Client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}()

	// 3. Store the pre-routed endpoint so the actual request skips the router
	a.warmPaths.Store(intent.SessionID, result.Endpoint)
}

// GetWarmedEndpoint returns the pre-selected endpoint for a session, if any.
func (a *AnticipatoryPrefetcher) GetWarmedEndpoint(sessionID string) (string, bool) {
	if ep, ok := a.warmPaths.LoadAndDelete(sessionID); ok {
		return ep.(string), true
	}
	return "", false
}

// --- Zero-Copy Network Config ---

// ZeroCopyConfig defines infrastructure settings required for GPU-Direct RDMA.
// When enabled, the operator provisions pods with SR-IOV network definitions
// allowing NCCL to bypass the CPU and host memory completely during tensor parallel sync.
type ZeroCopyConfig struct {
	// EnableRDMA injects Mellanox/Nvidia SR-IOV resources into the worker pods.
	EnableRDMA bool `json:"enableRDMA"`

	// ResourceName is the k8s extended resource for the RDMA VF (e.g., mellanox.com/cx5_sriov).
	ResourceName string `json:"resourceName"`

	// SharedMemorySize is the size of the /dev/shm mount for collective communication staging.
	SharedMemorySize string `json:"sharedMemorySize"` // e.g., "16Gi"
}

// ApplyZeroCopy applies RDMA/SR-IOV multi-net annotations to a pod template.
func (z *ZeroCopyConfig) ApplyZeroCopy(annotations, limits map[string]string) {
	if !z.EnableRDMA {
		return
	}

	// Multus CNI annotation for the secondary SR-IOV interface (roce-network)
	if annotations != nil {
		annotations["k8s.v1.cni.cncf.io/networks"] = "roce-network"
	}

	// Request the hardware Virtual Function (VF)
	if limits != nil && z.ResourceName != "" {
		limits[z.ResourceName] = "1"
	}
}
