/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package inference provides the request-level inference pipeline optimized
// for minimum time-to-first-token (TTFT) and time-to-inference (TTI).
package inference

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages persistent HTTP/2 connections to inference endpoints.
// Eliminates TCP+TLS handshake overhead on the hot path.
type ConnectionPool struct {
	mu        sync.RWMutex
	endpoints map[string]*EndpointConn
	transport *http.Transport
	config    PoolConfig
}

// PoolConfig configures the connection pool.
type PoolConfig struct {
	// MaxIdleConnsPerHost controls persistent connection reuse.
	MaxIdleConnsPerHost int

	// MaxConnsPerHost caps total connections to a single endpoint.
	MaxConnsPerHost int

	// IdleConnTimeout is how long idle connections stay in the pool.
	IdleConnTimeout time.Duration

	// DialTimeout is the TCP connection timeout.
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the TLS negotiation timeout.
	TLSHandshakeTimeout time.Duration

	// EnableHTTP2 forces HTTP/2 for multiplexed requests.
	EnableHTTP2 bool

	// DisableKeepAlives disables connection reuse (for testing only).
	DisableKeepAlives bool
}

// DefaultPoolConfig returns production defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     250,
		IdleConnTimeout:     90 * time.Second,
		DialTimeout:         2 * time.Second,
		TLSHandshakeTimeout: 3 * time.Second,
		EnableHTTP2:         true,
	}
}

// EndpointConn tracks a single endpoint's connection state.
type EndpointConn struct {
	// Address is the endpoint address (ip:port).
	Address string

	// Client is the per-endpoint HTTP client with connection pooling.
	Client *http.Client

	// ActiveRequests tracks in-flight requests for load awareness.
	ActiveRequests atomic.Int64

	// AvgLatencyMs is the exponentially weighted moving average latency.
	AvgLatencyMs atomic.Int64

	// TotalRequests is the lifetime request count.
	TotalRequests atomic.Int64

	// ErrorCount is the recent error count (reset on success).
	ErrorCount atomic.Int64

	// LastUsed tracks the last request time for idle eviction.
	LastUsed atomic.Int64
}

// NewConnectionPool creates a pool with optimized transport settings.
func NewConnectionPool(config PoolConfig) *ConnectionPool {
	transport := &http.Transport{
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		DisableKeepAlives:   config.DisableKeepAlives,
		ForceAttemptHTTP2:   config.EnableHTTP2,
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		// Pre-allocate write buffer to avoid allocation on hot path
		WriteBufferSize: 64 * 1024,
		ReadBufferSize:  64 * 1024,
	}

	return &ConnectionPool{
		endpoints: make(map[string]*EndpointConn),
		transport: transport,
		config:    config,
	}
}

// Get returns or creates a connection to the endpoint.
func (p *ConnectionPool) Get(address string) *EndpointConn {
	p.mu.RLock()
	conn, ok := p.endpoints[address]
	p.mu.RUnlock()
	if ok {
		return conn
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after write lock
	if conn, ok := p.endpoints[address]; ok {
		return conn
	}

	conn = &EndpointConn{
		Address: address,
		Client: &http.Client{
			Transport: p.transport,
			Timeout:   0, // Managed by LatencyBudget per-request
		},
	}
	p.endpoints[address] = conn
	return conn
}

// FastestEndpoint returns the endpoint with the lowest weighted latency.
// Combines avg latency, active request count, and error rate.
func (p *ConnectionPool) FastestEndpoint(candidates []string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bestAddr := ""
	bestScore := int64(1<<62 - 1) // max int63

	for _, addr := range candidates {
		conn, ok := p.endpoints[addr]
		if !ok {
			return addr // New endpoint, try it
		}

		// Score = avgLatency + (activeRequests * 10) + (errors * 100)
		score := conn.AvgLatencyMs.Load() +
			conn.ActiveRequests.Load()*10 +
			conn.ErrorCount.Load()*100

		if score < bestScore {
			bestScore = score
			bestAddr = addr
		}
	}

	if bestAddr == "" && len(candidates) > 0 {
		return candidates[0]
	}
	return bestAddr
}

// RecordLatency updates the EWMA latency for an endpoint.
func (p *ConnectionPool) RecordLatency(address string, latency time.Duration) {
	conn := p.Get(address)
	ms := latency.Milliseconds()

	// EWMA with alpha=0.3 (recent samples weighted more)
	current := conn.AvgLatencyMs.Load()
	if current == 0 {
		conn.AvgLatencyMs.Store(ms)
	} else {
		updated := (current*7 + ms*3) / 10
		conn.AvgLatencyMs.Store(updated)
	}

	conn.TotalRequests.Add(1)
	conn.ErrorCount.Store(0) // Reset on success
	conn.LastUsed.Store(time.Now().UnixMilli())
}

// RecordError increments the error count for circuit-breaking.
func (p *ConnectionPool) RecordError(address string) {
	conn := p.Get(address)
	conn.ErrorCount.Add(1)
}

// EvictIdle removes endpoints that haven't been used recently.
func (p *ConnectionPool) EvictIdle(maxIdle time.Duration) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-maxIdle).UnixMilli()
	evicted := 0

	for addr, conn := range p.endpoints {
		if conn.LastUsed.Load() < cutoff && conn.ActiveRequests.Load() == 0 {
			delete(p.endpoints, addr)
			evicted++
		}
	}
	return evicted
}
