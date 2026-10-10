/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package kvstate provides pure domain logic for the KV-cache and inference-state
// fabric. It models multi-tier cache residency, chunked prefix caching,
// adapter-aware cache identity, prefill-to-decode (P->D) transfer governance,
// and state reuse evidence without external transport or cluster dependencies.
package kvstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidIdentity       = errors.New("invalid cache identity")
	ErrTenantIsolationBreach = errors.New("tenant isolation boundary breach")
	ErrAdapterMismatch       = errors.New("adapter mismatch")
	ErrTokenizerMismatch     = errors.New("tokenizer mismatch")
	ErrPartitionMismatch     = errors.New("partition mismatch")
	ErrStateConflict         = errors.New("cache state conflict")
	ErrEntryNotFound         = errors.New("cache entry not found")
	ErrInvalidTierTransition = errors.New("invalid tier transition")
	ErrTransferFailed        = errors.New("kv transfer failed")
	ErrTransferTimeout       = errors.New("kv transfer timed out")
	ErrIntegrityViolation    = errors.New("kv cache integrity violation")
)

type ResidencyTier string

const (
	TierGPU       ResidencyTier = "tier-gpu"
	TierHostCPU   ResidencyTier = "tier-host-cpu"
	TierLocalDisk ResidencyTier = "tier-local-disk"
	TierRemote    ResidencyTier = "tier-remote"
	TierEvicted   ResidencyTier = "tier-evicted"
	TierCold      ResidencyTier = "tier-cold"
)

type LifecycleState string

const (
	LifecycleUnallocated LifecycleState = "unallocated"
	LifecycleAllocating  LifecycleState = "allocating"
	LifecycleActive      LifecycleState = "active"
	LifecycleDraining    LifecycleState = "draining"
	LifecycleEvicted     LifecycleState = "evicted"
	LifecycleQuarantined LifecycleState = "quarantined"
)

type AffinityPosture string

const (
	AffinityNone     AffinityPosture = "none"
	AffinitySoft     AffinityPosture = "soft"
	AffinityStrict   AffinityPosture = "strict"
	AffinityOrphaned AffinityPosture = "orphaned"
)

type IsolationPosture string

const (
	IsolationTenantStrict    IsolationPosture = "tenant-strict"
	IsolationPartitionStrict IsolationPosture = "partition-strict"
	IsolationSharedPreamble  IsolationPosture = "shared-preamble"
)

type IntegrityPosture string

const (
	IntegrityUnverified IntegrityPosture = "unverified"
	IntegrityVerified   IntegrityPosture = "verified"
	IntegrityCorrupted  IntegrityPosture = "corrupted"
)

type ReuseDisposition string

const (
	ReuseHit                     ReuseDisposition = "hit"
	ReuseMiss                    ReuseDisposition = "miss"
	ReusePartialHit              ReuseDisposition = "partial-hit"
	ReuseDeniedTenantMismatch    ReuseDisposition = "denied-tenant-mismatch"
	ReuseDeniedAdapterMismatch   ReuseDisposition = "denied-adapter-mismatch"
	ReuseDeniedTokenizerMismatch ReuseDisposition = "denied-tokenizer-mismatch"
	ReuseDeniedPartitionMismatch ReuseDisposition = "denied-partition-mismatch"
	ReuseDeniedExpired           ReuseDisposition = "denied-expired"
	ReuseDeniedQuarantined       ReuseDisposition = "denied-quarantined"
	ReuseDeniedCorrupted         ReuseDisposition = "denied-corrupted"
	ReuseDeniedNotActive         ReuseDisposition = "denied-not-active"
)

// StateVector encapsulates the multidimensional state tuple of a cache entry.
// Governed by CKODEX rule: Vector state, not booleans.
type StateVector struct {
	Residency ResidencyTier    `json:"residency"`
	Lifecycle LifecycleState   `json:"lifecycle"`
	Transfer  TransferPhase    `json:"transfer"`
	Affinity  AffinityPosture  `json:"affinity"`
	Isolation IsolationPosture `json:"isolation"`
	Integrity IntegrityPosture `json:"integrity"`
}

func (v StateVector) EvaluateReuse(now, expiresAt time.Time) ReuseDisposition {
	if v.Lifecycle == LifecycleQuarantined {
		return ReuseDeniedQuarantined
	}
	if v.Integrity == IntegrityCorrupted {
		return ReuseDeniedCorrupted
	}
	if !expiresAt.IsZero() && !now.Before(expiresAt) {
		return ReuseDeniedExpired
	}
	if v.Lifecycle == LifecycleEvicted || v.Residency == TierEvicted || v.Residency == TierCold {
		return ReuseMiss
	}
	if v.Lifecycle != LifecycleActive {
		return ReuseDeniedNotActive
	}
	switch v.Residency {
	case TierGPU, TierHostCPU, TierLocalDisk, TierRemote:
		return ReuseHit
	default:
		return ReuseMiss
	}
}

// CacheIdentity uniquely identifies KV-cache state across five orthogonal dimensions:
// (Tenant, Partition, Model, Adapter, Tokenizer) plus chunk sequence metadata.
type CacheIdentity struct {
	TenantID    string `json:"tenantId"`
	PartitionID string `json:"partitionId"`
	ModelID     string `json:"modelId"`
	AdapterID   string `json:"adapterId,omitempty"`
	TokenizerID string `json:"tokenizerId"`
	PrefixHash  string `json:"prefixHash"`
	ChunkIndex  int    `json:"chunkIndex"`
}

func (id CacheIdentity) Validate() error {
	if strings.TrimSpace(id.TenantID) == "" || strings.TrimSpace(id.PartitionID) == "" {
		return fmt.Errorf("%w: missing tenantId or partitionId", ErrInvalidIdentity)
	}
	if strings.TrimSpace(id.ModelID) == "" || strings.TrimSpace(id.TokenizerID) == "" {
		return fmt.Errorf("%w: missing modelId or tokenizerId", ErrInvalidIdentity)
	}
	if strings.TrimSpace(id.PrefixHash) == "" || id.ChunkIndex < 0 {
		return fmt.Errorf("%w: invalid prefixHash or chunkIndex", ErrInvalidIdentity)
	}
	return nil
}

func (id CacheIdentity) URN() string {
	return fmt.Sprintf("urn:ckodex:kv:%s:%s:%s:%s:%s:%d:%s",
		id.TenantID, id.PartitionID, id.ModelID, id.AdapterID,
		id.TokenizerID, id.ChunkIndex, id.PrefixHash)
}

func (id CacheIdentity) Digest() string {
	h := sha256.New()
	parts := []string{
		id.TenantID, id.PartitionID, id.ModelID, id.AdapterID,
		id.TokenizerID, strconv.Itoa(id.ChunkIndex), id.PrefixHash,
	}
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (id CacheIdentity) CheckCompatibility(query CacheIdentity) ReuseDisposition {
	if id.TenantID != query.TenantID {
		return ReuseDeniedTenantMismatch
	}
	if id.PartitionID != query.PartitionID {
		return ReuseDeniedPartitionMismatch
	}
	if id.ModelID != query.ModelID || id.AdapterID != query.AdapterID {
		return ReuseDeniedAdapterMismatch
	}
	if id.TokenizerID != query.TokenizerID {
		return ReuseDeniedTokenizerMismatch
	}
	if id.ChunkIndex != query.ChunkIndex || id.PrefixHash != query.PrefixHash {
		return ReuseMiss
	}
	return ReuseHit
}

type CacheEntry struct {
	Identity     CacheIdentity
	Vector       StateVector
	NodeAffinity string
	Tokens       int
	SizeBytes    int64
	CreatedAt    time.Time
	LastAccessed time.Time
	ExpiresAt    time.Time
	AccessCount  int64
}

type PrefixMatchResult struct {
	MatchedChunks        int
	TotalRequestedChunks int
	MatchedTokens        int
	BestTier             ResidencyTier
	Entries              []CacheEntry
	Disposition          ReuseDisposition
}

type ReuseEvidence struct {
	EvidenceID    string           `json:"evidenceId"`
	IdentityURN   string           `json:"identityUrn"`
	SubjectDigest string           `json:"subjectDigest"`
	Disposition   ReuseDisposition `json:"disposition"`
	StateVector   StateVector      `json:"stateVector"`
	TokensSaved   int              `json:"tokensSaved"`
	LatencyMs     int64            `json:"latencyMs"`
	RecordedAt    string           `json:"recordedAt"`
	Digest        string           `json:"digest"`
}

type evidenceClaims struct {
	EvidenceID    string           `json:"evidenceId"`
	IdentityURN   string           `json:"identityUrn"`
	SubjectDigest string           `json:"subjectDigest"`
	Disposition   ReuseDisposition `json:"disposition"`
	StateVector   StateVector      `json:"stateVector"`
	TokensSaved   int              `json:"tokensSaved"`
	LatencyMs     int64            `json:"latencyMs"`
	RecordedAt    string           `json:"recordedAt"`
}

type CacheFabric struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

func NewCacheFabric() *CacheFabric {
	return &CacheFabric{entries: make(map[string]CacheEntry)}
}

func (f *CacheFabric) Insert(entry CacheEntry) error {
	if err := entry.Identity.Validate(); err != nil {
		return fmt.Errorf("insert validation: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[entry.Identity.Digest()] = entry
	return nil
}

func (f *CacheFabric) Lookup(query CacheIdentity, now time.Time) (CacheEntry, ReuseDisposition, error) {
	if err := query.Validate(); err != nil {
		return CacheEntry{}, ReuseMiss, fmt.Errorf("lookup identity: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[query.Digest()]
	if !ok {
		return CacheEntry{}, ReuseMiss, nil
	}
	if disp := entry.Identity.CheckCompatibility(query); disp != ReuseHit {
		return CacheEntry{}, disp, nil
	}
	disp := entry.Vector.EvaluateReuse(now, entry.ExpiresAt)
	if disp == ReuseHit {
		entry.AccessCount++
		entry.LastAccessed = now
		f.entries[query.Digest()] = entry
	}
	return entry, disp, nil
}

func (f *CacheFabric) PrefixLookup(base CacheIdentity, hashes []string, now time.Time) PrefixMatchResult {
	res := PrefixMatchResult{TotalRequestedChunks: len(hashes), Disposition: ReuseMiss}
	for i, ph := range hashes {
		chunkID := base
		chunkID.ChunkIndex = i
		chunkID.PrefixHash = ph
		entry, disp, err := f.Lookup(chunkID, now)
		if err != nil || disp != ReuseHit {
			break
		}
		res.MatchedChunks++
		res.MatchedTokens += entry.Tokens
		res.Entries = append(res.Entries, entry)
		res.BestTier = selectBestTier(res.BestTier, entry.Vector.Residency)
	}
	if res.MatchedChunks == res.TotalRequestedChunks && res.TotalRequestedChunks > 0 {
		res.Disposition = ReuseHit
	} else if res.MatchedChunks > 0 {
		res.Disposition = ReusePartialHit
	}
	return res
}

func selectBestTier(current, next ResidencyTier) ResidencyTier {
	rank := map[ResidencyTier]int{
		TierGPU: 4, TierHostCPU: 3, TierLocalDisk: 2, TierRemote: 1, TierEvicted: 0, TierCold: 0,
	}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func (f *CacheFabric) Promote(id CacheIdentity, target ResidencyTier) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[id.Digest()]
	if !ok {
		return fmt.Errorf("%w: entry not found", ErrEntryNotFound)
	}
	if entry.Vector.Lifecycle != LifecycleActive {
		return fmt.Errorf("%w: cannot promote non-active entry", ErrInvalidTierTransition)
	}
	entry.Vector.Residency = target
	f.entries[id.Digest()] = entry
	return nil
}

func (f *CacheFabric) Demote(id CacheIdentity, target ResidencyTier) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[id.Digest()]
	if !ok {
		return fmt.Errorf("%w: entry not found", ErrEntryNotFound)
	}
	entry.Vector.Residency = target
	f.entries[id.Digest()] = entry
	return nil
}

func (f *CacheFabric) Evict(id CacheIdentity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[id.Digest()]
	if !ok {
		return fmt.Errorf("%w: entry not found", ErrEntryNotFound)
	}
	entry.Vector.Residency = TierEvicted
	entry.Vector.Lifecycle = LifecycleEvicted
	f.entries[id.Digest()] = entry
	return nil
}

func (f *CacheFabric) EvictExpired(now time.Time) []CacheIdentity {
	f.mu.Lock()
	defer f.mu.Unlock()
	var evicted []CacheIdentity
	for digest, entry := range f.entries {
		if !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) && entry.Vector.Lifecycle != LifecycleEvicted {
			entry.Vector.Residency = TierEvicted
			entry.Vector.Lifecycle = LifecycleEvicted
			f.entries[digest] = entry
			evicted = append(evicted, entry.Identity)
		}
	}
	sort.Slice(evicted, func(i, j int) bool { return evicted[i].URN() < evicted[j].URN() })
	return evicted
}

func (f *CacheFabric) RecordReuseEvidence(
	id CacheIdentity, disp ReuseDisposition, vec StateVector,
	tokensSaved int, latencySavedMs int64, now time.Time,
) (ReuseEvidence, error) {
	if err := id.Validate(); err != nil {
		return ReuseEvidence{}, fmt.Errorf("evidence identity: %w", err)
	}
	rfcNow := now.UTC().Format(time.RFC3339Nano)
	claims := evidenceClaims{
		EvidenceID:  fmt.Sprintf("ev-%d", now.UnixNano()),
		IdentityURN: id.URN(), SubjectDigest: id.Digest(),
		Disposition: disp, StateVector: vec,
		TokensSaved: tokensSaved, LatencyMs: latencySavedMs, RecordedAt: rfcNow,
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return ReuseEvidence{}, fmt.Errorf("marshal evidence claims: %w", err)
	}
	sum := sha256.Sum256(raw)
	return ReuseEvidence{
		EvidenceID: claims.EvidenceID, IdentityURN: claims.IdentityURN,
		SubjectDigest: claims.SubjectDigest, Disposition: claims.Disposition,
		StateVector: claims.StateVector, TokensSaved: claims.TokensSaved,
		LatencyMs: claims.LatencyMs, RecordedAt: claims.RecordedAt,
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}
