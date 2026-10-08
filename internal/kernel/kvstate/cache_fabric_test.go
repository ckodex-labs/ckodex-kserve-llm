/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package kvstate

import (
	"errors"
	"testing"
	"time"
)

func sampleIdentity(tenant, partition, model, adapter, tok, prefix string, chunk int) CacheIdentity {
	return CacheIdentity{
		TenantID:    tenant,
		PartitionID: partition,
		ModelID:     model,
		AdapterID:   adapter,
		TokenizerID: tok,
		PrefixHash:  prefix,
		ChunkIndex:  chunk,
	}
}

func sampleActiveVector(tier ResidencyTier) StateVector {
	return StateVector{
		Residency: tier,
		Lifecycle: LifecycleActive,
		Transfer:  TransferPhaseIdle,
		Affinity:  AffinitySoft,
		Isolation: IsolationTenantStrict,
		Integrity: IntegrityVerified,
	}
}

func TestCacheIdentity_UniquenessAcrossDimensions(t *testing.T) {
	base := sampleIdentity("tenant-a", "us-east-1", "llama-3-70b", "adapter-fin", "tiktoken", "hash-001", 0)
	if err := base.Validate(); err != nil {
		t.Fatalf("expected valid identity, got: %v", err)
	}

	variants := []CacheIdentity{
		sampleIdentity("tenant-b", "us-east-1", "llama-3-70b", "adapter-fin", "tiktoken", "hash-001", 0),
		sampleIdentity("tenant-a", "eu-west-1", "llama-3-70b", "adapter-fin", "tiktoken", "hash-001", 0),
		sampleIdentity("tenant-a", "us-east-1", "qwen-2.5-72b", "adapter-fin", "tiktoken", "hash-001", 0),
		sampleIdentity("tenant-a", "us-east-1", "llama-3-70b", "adapter-med", "tiktoken", "hash-001", 0),
		sampleIdentity("tenant-a", "us-east-1", "llama-3-70b", "adapter-fin", "hf-llama3", "hash-001", 0),
		sampleIdentity("tenant-a", "us-east-1", "llama-3-70b", "adapter-fin", "tiktoken", "hash-002", 0),
		sampleIdentity("tenant-a", "us-east-1", "llama-3-70b", "adapter-fin", "tiktoken", "hash-001", 1),
	}

	digests := make(map[string]bool)
	digests[base.Digest()] = true

	for i, v := range variants {
		if err := v.Validate(); err != nil {
			t.Fatalf("variant %d failed validation: %v", i, err)
		}
		d := v.Digest()
		if digests[d] {
			t.Fatalf("collision detected for variant %d: %s", i, d)
		}
		digests[d] = true
		if v.URN() == base.URN() {
			t.Fatalf("URN collision for variant %d", i)
		}
	}
}

func TestCacheIdentity_Validation(t *testing.T) {
	cases := []struct {
		name string
		id   CacheIdentity
	}{
		{"empty tenant", sampleIdentity("", "p", "m", "a", "tok", "h", 0)},
		{"empty partition", sampleIdentity("t", "", "m", "a", "tok", "h", 0)},
		{"empty model", sampleIdentity("t", "p", "", "a", "tok", "h", 0)},
		{"empty tokenizer", sampleIdentity("t", "p", "m", "a", "", "h", 0)},
		{"empty prefix", sampleIdentity("t", "p", "m", "a", "tok", "", 0)},
		{"negative chunk", sampleIdentity("t", "p", "m", "a", "tok", "h", -1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.id.Validate(); !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("expected ErrInvalidIdentity, got: %v", err)
			}
		})
	}
}

func TestTenantIsolation_PreventsCrossTenantReuse(t *testing.T) {
	fabric := NewCacheFabric()
	now := time.Now().UTC()

	idTenantA := sampleIdentity("tenant-alpha", "zone-1", "llama-3", "", "tok-1", "prefix-common", 0)
	idTenantB := sampleIdentity("tenant-bravo", "zone-1", "llama-3", "", "tok-1", "prefix-common", 0)

	entryA := CacheEntry{
		Identity:     idTenantA,
		Vector:       sampleActiveVector(TierGPU),
		Tokens:       256,
		CreatedAt:    now,
		LastAccessed: now,
	}
	if err := fabric.Insert(entryA); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	_, dispA, err := fabric.Lookup(idTenantA, now)
	if err != nil || dispA != ReuseHit {
		t.Fatalf("tenant-alpha should hit own cache, got %s, err: %v", dispA, err)
	}

	_, dispB, err := fabric.Lookup(idTenantB, now)
	if err != nil || dispB != ReuseMiss {
		t.Fatalf("tenant-bravo should miss tenant-alpha's cache, got %s, err: %v", dispB, err)
	}

	compat := idTenantA.CheckCompatibility(idTenantB)
	if compat != ReuseDeniedTenantMismatch {
		t.Fatalf("expected ReuseDeniedTenantMismatch, got %s", compat)
	}
}

func TestAdapterAware_CacheIsolation(t *testing.T) {
	idBase := sampleIdentity("tenant-1", "zone-1", "llama-3", "", "tok-1", "pref-1", 0)
	idAdapter1 := sampleIdentity("tenant-1", "zone-1", "llama-3", "lora-sales", "tok-1", "pref-1", 0)
	idAdapter2 := sampleIdentity("tenant-1", "zone-1", "llama-3", "lora-support", "tok-1", "pref-1", 0)

	if compat := idBase.CheckCompatibility(idAdapter1); compat != ReuseDeniedAdapterMismatch {
		t.Fatalf("expected adapter mismatch between base and adapter, got %s", compat)
	}
	if compat := idAdapter1.CheckCompatibility(idAdapter2); compat != ReuseDeniedAdapterMismatch {
		t.Fatalf("expected adapter mismatch between distinct adapters, got %s", compat)
	}
}

func TestTokenizerMismatch_Prevention(t *testing.T) {
	idTok1 := sampleIdentity("tenant-1", "zone-1", "llama-3", "", "tiktoken-cl100k", "pref-1", 0)
	idTok2 := sampleIdentity("tenant-1", "zone-1", "llama-3", "", "hf-llama3-tok", "pref-1", 0)

	if compat := idTok1.CheckCompatibility(idTok2); compat != ReuseDeniedTokenizerMismatch {
		t.Fatalf("expected tokenizer mismatch, got %s", compat)
	}
}

func TestPrefixCaching_MultiChunkMatching(t *testing.T) {
	fabric := NewCacheFabric()
	now := time.Now().UTC()
	base := sampleIdentity("tenant-1", "zone-1", "llama-3", "", "tok-1", "", 0)
	hashes := []string{"chunk0-hash", "chunk1-hash", "chunk2-hash", "chunk3-hash"}

	// Insert only first 2 chunks
	for i := 0; i < 2; i++ {
		id := base
		id.ChunkIndex = i
		id.PrefixHash = hashes[i]
		tier := TierHostCPU
		if i == 0 {
			tier = TierGPU
		}
		_ = fabric.Insert(CacheEntry{
			Identity:  id,
			Vector:    sampleActiveVector(tier),
			Tokens:    256,
			CreatedAt: now,
		})
	}

	partial := fabric.PrefixLookup(base, hashes, now)
	if partial.MatchedChunks != 2 || partial.MatchedTokens != 512 || partial.Disposition != ReusePartialHit {
		t.Fatalf("expected partial hit of 2 chunks / 512 tokens, got: %+v", partial)
	}
	if partial.BestTier != TierGPU {
		t.Fatalf("expected BestTier TierGPU, got %s", partial.BestTier)
	}

	// Insert remaining chunks
	for i := 2; i < 4; i++ {
		id := base
		id.ChunkIndex = i
		id.PrefixHash = hashes[i]
		_ = fabric.Insert(CacheEntry{
			Identity:  id,
			Vector:    sampleActiveVector(TierHostCPU),
			Tokens:    256,
			CreatedAt: now,
		})
	}

	full := fabric.PrefixLookup(base, hashes, now)
	if full.MatchedChunks != 4 || full.MatchedTokens != 1024 || full.Disposition != ReuseHit {
		t.Fatalf("expected full hit of 4 chunks, got: %+v", full)
	}
}

func TestTieredResidency_TransitionsAndEviction(t *testing.T) {
	fabric := NewCacheFabric()
	now := time.Now().UTC()
	id := sampleIdentity("tenant-1", "zone-1", "llama-3", "", "tok-1", "p-hash", 0)

	entry := CacheEntry{
		Identity:  id,
		Vector:    sampleActiveVector(TierRemote),
		Tokens:    128,
		CreatedAt: now,
	}
	if err := fabric.Insert(entry); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if err := fabric.Promote(id, TierHostCPU); err != nil {
		t.Fatalf("promote to HostCPU failed: %v", err)
	}
	got, disp, _ := fabric.Lookup(id, now)
	if disp != ReuseHit || got.Vector.Residency != TierHostCPU {
		t.Fatalf("expected HostCPU hit, got tier: %s, disp: %s", got.Vector.Residency, disp)
	}

	if err := fabric.Promote(id, TierGPU); err != nil {
		t.Fatalf("promote to GPU failed: %v", err)
	}
	got, _, _ = fabric.Lookup(id, now)
	if got.Vector.Residency != TierGPU {
		t.Fatalf("expected TierGPU, got: %s", got.Vector.Residency)
	}

	if err := fabric.Demote(id, TierRemote); err != nil {
		t.Fatalf("demote failed: %v", err)
	}
	if err := fabric.Evict(id); err != nil {
		t.Fatalf("evict failed: %v", err)
	}
	_, disp, _ = fabric.Lookup(id, now)
	if disp != ReuseMiss {
		t.Fatalf("expected miss after eviction, got: %s", disp)
	}
}

func TestEvictionTTL_Sweep(t *testing.T) {
	fabric := NewCacheFabric()
	baseTime := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	liveID := sampleIdentity("t1", "z1", "m1", "", "tok", "live-hash", 0)
	expiredID := sampleIdentity("t1", "z1", "m1", "", "tok", "expired-hash", 0)

	_ = fabric.Insert(CacheEntry{
		Identity:  liveID,
		Vector:    sampleActiveVector(TierGPU),
		ExpiresAt: baseTime.Add(10 * time.Minute),
	})
	_ = fabric.Insert(CacheEntry{
		Identity:  expiredID,
		Vector:    sampleActiveVector(TierGPU),
		ExpiresAt: baseTime.Add(-1 * time.Minute),
	})

	// Lookup expired directly
	_, disp, _ := fabric.Lookup(expiredID, baseTime)
	if disp != ReuseDeniedExpired {
		t.Fatalf("expected ReuseDeniedExpired, got: %s", disp)
	}

	evicted := fabric.EvictExpired(baseTime)
	if len(evicted) != 1 || evicted[0].Digest() != expiredID.Digest() {
		t.Fatalf("expected 1 expired entry swept, got: %+v", evicted)
	}
}

func TestTransferContract_PDDisaggregation(t *testing.T) {
	now := time.Now().UTC()
	id := sampleIdentity("t1", "z1", "llama-3", "lora-code", "tok", "seq-hash", 0)
	deadline := now.Add(5 * time.Second)

	contract, err := NewTransferContract(
		"tx-001", id, "prefill-pod-1", "decode-pod-2", "lmcache",
		RoleProducer, 512, 1048576, deadline,
	)
	if err != nil {
		t.Fatalf("contract creation failed: %v", err)
	}

	if err := contract.Initiate(now); err != nil {
		t.Fatalf("initiate failed: %v", err)
	}
	if err := contract.MarkInFlight(now.Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("in flight failed: %v", err)
	}
	if err := contract.Complete(now.Add(200*time.Millisecond), "sha256:abc123def"); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if contract.Phase != TransferPhaseCompleted {
		t.Fatalf("expected completed phase, got: %s", contract.Phase)
	}

	// Governance check on target node adapter
	if err := contract.VerifyGovernance("lora-code"); err != nil {
		t.Fatalf("expected valid governance, got: %v", err)
	}
	if err := contract.VerifyGovernance("lora-other"); !errors.Is(err, ErrAdapterMismatch) {
		t.Fatalf("expected adapter mismatch on target decode node, got: %v", err)
	}
}

func TestTransferContract_TimeoutAndIntegrityViolations(t *testing.T) {
	now := time.Now().UTC()
	id := sampleIdentity("t1", "z1", "llama-3", "", "tok", "seq-hash", 0)
	deadline := now.Add(50 * time.Millisecond)

	contract, _ := NewTransferContract(
		"tx-002", id, "prefill-1", "decode-1", "nixl",
		RoleProducer, 128, 65536, deadline,
	)
	_ = contract.Initiate(now)

	// Past deadline
	err := contract.MarkInFlight(now.Add(100 * time.Millisecond))
	if !errors.Is(err, ErrTransferTimeout) {
		t.Fatalf("expected ErrTransferTimeout, got: %v", err)
	}

	// In-flight without checksum
	c2, _ := NewTransferContract("tx-003", id, "p1", "d1", "nixl", RoleProducer, 128, 65536, now.Add(time.Hour))
	_ = c2.Initiate(now)
	_ = c2.MarkInFlight(now)
	err = c2.Complete(now, "")
	if !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("expected ErrIntegrityViolation for empty checksum, got: %v", err)
	}
}

func TestReuseEvidence_IntegrityDigest(t *testing.T) {
	fabric := NewCacheFabric()
	now := time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC)
	id := sampleIdentity("tenant-audit", "sec-zone", "llama-3", "", "tok", "audit-pref", 0)
	vec := sampleActiveVector(TierGPU)

	ev, err := fabric.RecordReuseEvidence(id, ReuseHit, vec, 512, 85, now)
	if err != nil {
		t.Fatalf("evidence recording failed: %v", err)
	}
	if ev.Digest == "" || ev.SubjectDigest != id.Digest() {
		t.Fatalf("expected valid digests in evidence: %+v", ev)
	}
	if ev.TokensSaved != 512 || ev.LatencyMs != 85 {
		t.Fatalf("expected metric fields recorded: %+v", ev)
	}

	// Invalid identity on evidence recording
	badID := id
	badID.TenantID = ""
	if _, err := fabric.RecordReuseEvidence(badID, ReuseHit, vec, 0, 0, now); err == nil {
		t.Fatalf("expected error on invalid identity for evidence")
	}
}

func TestStateVector_Dispositions(t *testing.T) {
	now := time.Now().UTC()
	vQuarantined := StateVector{Lifecycle: LifecycleQuarantined}
	if d := vQuarantined.EvaluateReuse(now, time.Time{}); d != ReuseDeniedQuarantined {
		t.Fatalf("expected ReuseDeniedQuarantined, got %s", d)
	}
	vCorrupted := StateVector{Integrity: IntegrityCorrupted}
	if d := vCorrupted.EvaluateReuse(now, time.Time{}); d != ReuseDeniedCorrupted {
		t.Fatalf("expected ReuseDeniedCorrupted, got %s", d)
	}
	vDraining := StateVector{Lifecycle: LifecycleDraining, Residency: TierGPU}
	if d := vDraining.EvaluateReuse(now, time.Time{}); d != ReuseDeniedNotActive {
		t.Fatalf("expected ReuseDeniedNotActive, got %s", d)
	}
	vCold := StateVector{Lifecycle: LifecycleActive, Residency: TierCold}
	if d := vCold.EvaluateReuse(now, time.Time{}); d != ReuseMiss {
		t.Fatalf("expected ReuseMiss, got %s", d)
	}
}

func TestTransferContract_EdgeTransitions(t *testing.T) {
	now := time.Now().UTC()
	id := sampleIdentity("t1", "z1", "llama-3", "", "tok", "seq", 0)
	c, _ := NewTransferContract("tx", id, "src", "dst", "lmcache", RoleBoth, 10, 10, now.Add(time.Hour))

	// Mark in-flight without initiate
	if err := c.MarkInFlight(now); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("expected ErrStateConflict, got: %v", err)
	}
	_ = c.Initiate(now)
	if err := c.Initiate(now); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("expected ErrStateConflict on double initiate, got: %v", err)
	}
	// Complete before in-flight
	if err := c.Complete(now, "sum"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("expected ErrStateConflict, got: %v", err)
	}
	// Fail method
	c.Fail(now, "manual abort")
	if c.Phase != TransferPhaseFailed || c.FailureReason != "manual abort" {
		t.Fatalf("expected failed phase, got: %+v", c)
	}

	// NewTransferContract validation errors
	if _, err := NewTransferContract("tx", id, "same", "same", "conn", RoleBoth, 1, 1, now); err == nil {
		t.Fatalf("expected error on same src and dst")
	}
	if _, err := NewTransferContract("", id, "s", "d", "c", RoleBoth, 1, 1, now); err == nil {
		t.Fatalf("expected error on empty id")
	}
}

func TestFabric_ErrorsAndMissingEntries(t *testing.T) {
	fabric := NewCacheFabric()
	now := time.Now().UTC()
	id := sampleIdentity("t1", "z1", "m1", "", "tok", "p", 0)

	// Missing entry lookups / modifications
	if err := fabric.Promote(id, TierGPU); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
	if err := fabric.Demote(id, TierRemote); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
	if err := fabric.Evict(id); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}

	// Invalid identity on lookup and insert
	badID := id
	badID.ModelID = ""
	if _, _, err := fabric.Lookup(badID, now); err == nil {
		t.Fatalf("expected error on bad lookup id")
	}
	if err := fabric.Insert(CacheEntry{Identity: badID}); err == nil {
		t.Fatalf("expected error on bad insert id")
	}
}
