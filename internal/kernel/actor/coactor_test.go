package actor

import (
	"errors"
	"testing"
	"time"
)

func TestActorLifecycleAndResidency(t *testing.T) {
	now := time.Now()
	a := NewActor("model-1", ActorTypeModel, 1, ReentrancyForbidden, 5*time.Minute, now)
	if sv := a.StateVector(); sv.Lifecycle != ActorInactive || sv.Residency != ResidencyCold {
		t.Fatalf("expected Inactive/Cold, got %+v", sv)
	}
	if err := a.AcquireSlot(now); !errors.Is(err, ErrActorNotActive) {
		t.Fatalf("expected ErrActorNotActive, got %v", err)
	}
	if err := a.Activate(now); err != nil {
		t.Fatalf("unexpected activation error: %v", err)
	}
	if err := a.AcquireSlot(now); !errors.Is(err, ErrActorNotResident) {
		t.Fatalf("expected ErrActorNotResident, got %v", err)
	}
	_ = a.SetResidency(ResidencyResident)
	if err := a.AcquireSlot(now); err != nil {
		t.Fatalf("slot acquire failed: %v", err)
	}
	if err := a.Deactivate(now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition deactivating active actor, got %v", err)
	}
	_ = a.ReleaseSlot(now)
	if err := a.ReleaseSlot(now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition on underflow, got %v", err)
	}
	if err := a.Deactivate(now); err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}
	if sv := a.StateVector(); sv.Lifecycle != ActorInactive {
		t.Fatalf("expected Inactive, got %s", sv.Lifecycle)
	}
}

func TestActorConcurrencyAndCapacity(t *testing.T) {
	now := time.Now()
	a := NewActor("strict", ActorTypeModel, 1, ReentrancyForbidden, 0, now)
	_ = a.Activate(now)
	_ = a.SetResidency(ResidencyResident)
	if err := a.AcquireSlot(now); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := a.AcquireSlot(now); !errors.Is(err, ErrReentrancyProhibited) {
		t.Fatalf("expected ErrReentrancyProhibited, got %v", err)
	}

	reentrant := NewActor("reentrant", ActorTypeModel, 2, ReentrancyAllowed, 0, now)
	_ = reentrant.Activate(now)
	_ = reentrant.SetResidency(ResidencyResident)
	_ = reentrant.AcquireSlot(now)
	if sv := reentrant.StateVector(); sv.Concurrency != ConcurrencyProcessing {
		t.Fatalf("expected Processing for 1/2 slots, got %s", sv.Concurrency)
	}
	_ = reentrant.AcquireSlot(now)
	if sv := reentrant.StateVector(); sv.Concurrency != ConcurrencyAtCapacity {
		t.Fatalf("expected AtCapacity for 2/2 slots, got %s", sv.Concurrency)
	}
	if err := reentrant.AcquireSlot(now); !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("expected ErrCapacityExceeded, got %v", err)
	}
}

func TestActorIdleTimeout(t *testing.T) {
	now := time.Now()
	a := NewActor("agent-1", ActorTypeAgent, 1, ReentrancyAllowed, 10*time.Second, now)
	_ = a.Activate(now)
	status, err := a.CheckIdle(now.Add(5 * time.Second))
	if err != nil || status != IdleStatusActive {
		t.Fatalf("expected active before timeout, got %s, err: %v", status, err)
	}
	status, err = a.CheckIdle(now.Add(11 * time.Second))
	if err != nil || status != IdleStatusTimedOut {
		t.Fatalf("expected timed out, got %s, err: %v", status, err)
	}
	if sv := a.StateVector(); sv.Lifecycle != ActorInactive {
		t.Fatalf("expected Inactive after idle timeout, got %s", sv.Lifecycle)
	}
}

func TestInferenceSessionLifecycleAndTurns(t *testing.T) {
	now := time.Now()
	sess := NewInferenceSession("sess-1", "llama-3", "actor-1", "group-1", 30*time.Minute, 2, now)
	if sv := sess.StateVector(); sv.Phase != SessionPhaseActive || sv.Affinity != AffinityUnbound {
		t.Fatalf("expected Active/Unbound, got %+v", sv)
	}
	if err := sess.BindEndpoint("10.0.0.1:8000"); err != nil {
		t.Fatalf("bind endpoint failed: %v", err)
	}
	if sess.BoundEndpoint() != "10.0.0.1:8000" {
		t.Fatalf("expected bound endpoint, got %s", sess.BoundEndpoint())
	}
	if err := sess.RecordTurn(120, 1024*1024, now); err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	if sess.TurnCount() != 1 {
		t.Fatalf("expected turn count 1, got %d", sess.TurnCount())
	}
	err := sess.RecordTurn(80, 2048*1024, now.Add(time.Second))
	if !errors.Is(err, ErrMaxTurnsReached) {
		t.Fatalf("expected ErrMaxTurnsReached, got %v", err)
	}
	if sv := sess.StateVector(); sv.Phase != SessionPhaseCompleted || sv.Affinity != AffinityUnbound {
		t.Fatalf("expected Completed/Unbound after max turns, got %+v", sv)
	}
	if err := sess.RecordTurn(50, 0, now); !errors.Is(err, ErrSessionTerminal) {
		t.Fatalf("expected ErrSessionTerminal, got %v", err)
	}
}

func TestInferenceSessionExpiry(t *testing.T) {
	now := time.Now()
	sess := NewInferenceSession("sess-exp", "llama-3", "actor-1", "", 10*time.Minute, 0, now)
	if st := sess.CheckExpiry(now.Add(5 * time.Minute)); st != SessionLive {
		t.Fatalf("expected SessionLive, got %s", st)
	}
	if st := sess.CheckExpiry(now.Add(11 * time.Minute)); st != SessionExpired {
		t.Fatalf("expected SessionExpired, got %s", st)
	}
	if sv := sess.StateVector(); sv.Phase != SessionPhaseEvicted || sv.Cache != CacheEvicted {
		t.Fatalf("expected Evicted state vector, got %+v", sv)
	}
}

func setupPrefillDecodeGroup(t *testing.T) (*CoActorGroup, *Actor, *Actor) {
	t.Helper()
	now := time.Now()
	group := NewCoActorGroup("group-pd", PatternPrefillDecode)
	prefill := NewActor("prefill-1", ActorTypeModel, 2, ReentrancyAllowed, 0, now)
	_ = prefill.Activate(now)
	_ = prefill.SetResidency(ResidencyResident)
	decode := NewActor("decode-1", ActorTypeModel, 2, ReentrancyAllowed, 0, now)
	_ = decode.Activate(now)
	_ = decode.SetResidency(ResidencyResident)
	group.AddMember("p1", RolePrefill, prefill, nil)
	group.AddMember("d1", RoleDecode, decode, []string{"p1"})
	if phase := group.RefreshPhase(); phase != GroupPhaseReady {
		t.Fatalf("expected GroupPhaseReady, got %s", phase)
	}
	return group, prefill, decode
}

func TestDisaggregatedPrefillDecodeHandoff_Success(t *testing.T) {
	group, prefill, decode := setupPrefillDecodeGroup(t)
	now := time.Now()
	sess := NewInferenceSession("sess-pd", "llama-3", "", group.id, 15*time.Minute, 10, now)

	if err := group.InitiatePrefill(sess); err != nil {
		t.Fatalf("initiate prefill failed: %v", err)
	}
	if group.StateVector().Transfer != TransferPrefilling || sess.StateVector().Cache != CacheWarming {
		t.Fatalf("invalid prefill transfer state: %+v", group.StateVector())
	}
	digest := "sha256-kvcache-abc123xyz"
	fence, err := group.CreateTransferFence(sess, 512, digest, 30*time.Second, now)
	if err != nil || fence.Epoch != 1 || fence.Digest != digest {
		t.Fatalf("unexpected fence: %+v err: %v", fence, err)
	}
	if group.StateVector().Transfer != TransferFenced || sess.StateVector().Cache != CacheFenced {
		t.Fatalf("expected Fenced state, got %+v", group.StateVector())
	}
	if err := group.BeginTransfer(fence, TransferNCCL); err != nil {
		t.Fatalf("begin transfer failed: %v", err)
	}
	if group.StateVector().Transfer != TransferTransmitting {
		t.Fatalf("expected Transmitting, got %s", group.StateVector().Transfer)
	}
	err = group.VerifyAndCompleteHandoff(fence, digest, "10.0.0.2:8000", now.Add(time.Second), sess)
	if err != nil {
		t.Fatalf("handoff completion failed: %v", err)
	}
	if prefill.StateVector().Concurrency != ConcurrencyIdle || decode.StateVector().Concurrency != ConcurrencyProcessing {
		t.Fatalf("concurrency mismatch post handoff: prefill=%s decode=%s", prefill.StateVector().Concurrency, decode.StateVector().Concurrency)
	}
	if sess.BoundEndpoint() != "10.0.0.2:8000" || sess.StateVector().Cache != CacheTransferred {
		t.Fatalf("endpoint=%s cache=%s", sess.BoundEndpoint(), sess.StateVector().Cache)
	}
	if group.StateVector().Transfer != TransferCompleted {
		t.Fatalf("expected TransferCompleted, got %s", group.StateVector().Transfer)
	}
}

func TestDisaggregatedPrefillDecodeHandoff_DigestMismatch(t *testing.T) {
	group, _, _ := setupPrefillDecodeGroup(t)
	now := time.Now()
	sess := NewInferenceSession("sess-mismatch", "llama-3", "", group.id, 15*time.Minute, 10, now)
	_ = group.InitiatePrefill(sess)
	fence, _ := group.CreateTransferFence(sess, 256, "valid-digest", 30*time.Second, now)
	_ = group.BeginTransfer(fence, TransferRDMA)
	err := group.VerifyAndCompleteHandoff(fence, "corrupt-digest", "10.0.0.2:8000", now.Add(time.Second), sess)
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch, got %v", err)
	}
	if group.StateVector().Transfer != TransferFailed || sess.StateVector().Cache != CacheEmpty {
		t.Fatalf("expected TransferFailed and CacheEmpty on mismatch, got %+v", group.StateVector())
	}
}

func TestDisaggregatedPrefillDecodeHandoff_FenceExpired(t *testing.T) {
	group, _, _ := setupPrefillDecodeGroup(t)
	now := time.Now()
	sess := NewInferenceSession("sess-expired-fence", "llama-3", "", group.id, 15*time.Minute, 10, now)
	_ = group.InitiatePrefill(sess)
	fence, _ := group.CreateTransferFence(sess, 256, "digest", 2*time.Second, now)
	_ = group.BeginTransfer(fence, TransferTCP)
	err := group.VerifyAndCompleteHandoff(fence, "digest", "10.0.0.2:8000", now.Add(5*time.Second), sess)
	if !errors.Is(err, ErrFenceExpired) {
		t.Fatalf("expected ErrFenceExpired, got %v", err)
	}
	if group.StateVector().Transfer != TransferFailed {
		t.Fatalf("expected TransferFailed on expired fence, got %s", group.StateVector().Transfer)
	}
}

func TestDisaggregatedPrefillDecodeHandoff_StaleFenceAndAbort(t *testing.T) {
	group, _, _ := setupPrefillDecodeGroup(t)
	now := time.Now()
	sess := NewInferenceSession("sess-stale", "llama-3", "", group.id, 15*time.Minute, 10, now)
	_ = group.InitiatePrefill(sess)
	fence1, _ := group.CreateTransferFence(sess, 128, "d1", 10*time.Second, now)
	fence2, _ := group.CreateTransferFence(sess, 256, "d2", 10*time.Second, now)
	if fence2.Epoch <= fence1.Epoch {
		t.Fatalf("expected strictly monotonic epoch, got %d <= %d", fence2.Epoch, fence1.Epoch)
	}
	if err := group.BeginTransfer(fence1, TransferNCCL); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("expected ErrStaleFence for old fence, got %v", err)
	}
	if err := group.AbortTransfer(fence2, "prefill node preempted"); err != nil {
		t.Fatalf("abort transfer failed: %v", err)
	}
	if group.StateVector().Transfer != TransferAborted || group.StateVector().Phase != GroupPhaseDegraded {
		t.Fatalf("expected TransferAborted and GroupPhaseDegraded, got %+v", group.StateVector())
	}
}

func TestCoActorGroupPhases(t *testing.T) {
	now := time.Now()
	group := NewCoActorGroup("ensemble-1", PatternAgentEnsemble)
	if group.RefreshPhase() != GroupPhaseDissolved {
		t.Fatalf("expected Dissolved for empty group")
	}
	a1 := NewActor("agent-1", ActorTypeAgent, 1, ReentrancyAllowed, 0, now)
	a2 := NewActor("agent-2", ActorTypeAgent, 1, ReentrancyAllowed, 0, now)
	group.AddMember("m1", RoleWorker, a1, nil)
	group.AddMember("m2", RoleWorker, a2, nil)
	if phase := group.RefreshPhase(); phase != GroupPhaseForming {
		t.Fatalf("expected Forming when inactive, got %s", phase)
	}
	_ = a1.Activate(now)
	_ = a1.SetResidency(ResidencyResident)
	if phase := group.RefreshPhase(); phase != GroupPhaseDegraded {
		t.Fatalf("expected Degraded when partially ready, got %s", phase)
	}
	_ = a2.Activate(now)
	_ = a2.SetResidency(ResidencyResident)
	if phase := group.RefreshPhase(); phase != GroupPhaseReady {
		t.Fatalf("expected Ready when all members ready, got %s", phase)
	}
}
