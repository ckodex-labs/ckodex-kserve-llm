// Package actor implements pure domain state machines for Actor lifecycle,
// InferenceSession context management, and CoActorGroup collaborative coordination.
// It maintains zero external SDK dependencies and adheres strictly to pure kernel boundaries.
package actor

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidTransition    = errors.New("invalid state transition")
	ErrCapacityExceeded     = errors.New("actor capacity exceeded")
	ErrReentrancyProhibited = errors.New("reentrancy is prohibited")
	ErrActorNotActive       = errors.New("actor is not in active state")
	ErrActorNotResident     = errors.New("actor is not in resident state")
	ErrSessionExpired       = errors.New("inference session has expired")
	ErrSessionTerminal      = errors.New("inference session is in terminal state")
	ErrMaxTurnsReached      = errors.New("inference session max turns reached")
	ErrStaleFence           = errors.New("stale or invalid transfer fence epoch")
	ErrFenceExpired         = errors.New("transfer fence has expired")
	ErrDigestMismatch       = errors.New("transfer state digest mismatch")
	ErrMemberNotFound       = errors.New("required member role not found in coactor group")
	ErrInvalidPattern       = errors.New("unsupported pattern for this operation")
)

type (
	ActorLifecycle       string
	ActorResidency       string
	ConcurrencyState     string
	ReentrancyMode       string
	ActorType            string
	IdleStatus           string
	SessionPhase         string
	SessionCacheState    string
	SessionAffinityState string
	SessionExpiryStatus  string
	CoactorPattern       string
	CoactorGroupPhase    string
	TransferPhase        string
	TransferMechanism    string
	MemberRole           string
)

const (
	ActorInactive          ActorLifecycle       = "Inactive"
	ActorActivating        ActorLifecycle       = "Activating"
	ActorActive            ActorLifecycle       = "Active"
	ActorDeactivating      ActorLifecycle       = "Deactivating"
	ResidencyCold          ActorResidency       = "Cold"
	ResidencyLoading       ActorResidency       = "Loading"
	ResidencyResident      ActorResidency       = "Resident"
	ResidencyEvicting      ActorResidency       = "Evicting"
	ConcurrencyIdle        ConcurrencyState     = "Idle"
	ConcurrencyProcessing  ConcurrencyState     = "Processing"
	ConcurrencyAtCapacity  ConcurrencyState     = "AtCapacity"
	ReentrancyForbidden    ReentrancyMode       = "Forbidden"
	ReentrancyAllowed      ReentrancyMode       = "Allowed"
	ActorTypeModel         ActorType            = "model"
	ActorTypeAgent         ActorType            = "agent"
	ActorTypeTool          ActorType            = "tool"
	IdleStatusActive       IdleStatus           = "Active"
	IdleStatusTimedOut     IdleStatus           = "TimedOut"
	SessionPhaseActive     SessionPhase         = "Active"
	SessionPhaseIdle       SessionPhase         = "Idle"
	SessionPhaseDraining   SessionPhase         = "Draining"
	SessionPhaseEvicted    SessionPhase         = "Evicted"
	SessionPhaseCompleted  SessionPhase         = "Completed"
	CacheEmpty             SessionCacheState    = "Empty"
	CacheWarming           SessionCacheState    = "Warming"
	CacheResident          SessionCacheState    = "Resident"
	CacheFenced            SessionCacheState    = "Fenced"
	CacheTransferred       SessionCacheState    = "Transferred"
	CacheEvicted           SessionCacheState    = "Evicted"
	AffinityUnbound        SessionAffinityState = "Unbound"
	AffinityBound          SessionAffinityState = "Bound"
	AffinityRebinding      SessionAffinityState = "Rebinding"
	SessionLive            SessionExpiryStatus  = "Live"
	SessionExpired         SessionExpiryStatus  = "Expired"
	PatternPrefillDecode   CoactorPattern       = "prefill-decode"
	PatternAgentEnsemble   CoactorPattern       = "agent-ensemble"
	PatternPipeline        CoactorPattern       = "pipeline"
	PatternMixtureOfAgents CoactorPattern       = "mixture-of-agents"
	GroupPhaseForming      CoactorGroupPhase    = "Forming"
	GroupPhaseReady        CoactorGroupPhase    = "Ready"
	GroupPhaseDegraded     CoactorGroupPhase    = "Degraded"
	GroupPhaseDissolved    CoactorGroupPhase    = "Dissolved"
	TransferUninitialized  TransferPhase        = "Uninitialized"
	TransferPrefilling     TransferPhase        = "Prefilling"
	TransferFenced         TransferPhase        = "Fenced"
	TransferTransmitting   TransferPhase        = "Transmitting"
	TransferVerifying      TransferPhase        = "Verifying"
	TransferCompleted      TransferPhase        = "Completed"
	TransferFailed         TransferPhase        = "Failed"
	TransferAborted        TransferPhase        = "Aborted"
	TransferNCCL           TransferMechanism    = "nccl"
	TransferRDMA           TransferMechanism    = "rdma"
	TransferTCP            TransferMechanism    = "tcp"
	RolePrefill            MemberRole           = "prefill"
	RoleDecode             MemberRole           = "decode"
	RoleRouter             MemberRole           = "router"
	RoleWorker             MemberRole           = "worker"
)

type ActorStateVector struct {
	Lifecycle   ActorLifecycle
	Residency   ActorResidency
	Concurrency ConcurrencyState
}

type Actor struct {
	mu             sync.RWMutex
	id             string
	actorType      ActorType
	vector         ActorStateVector
	maxConcurrency int32
	activeSlots    int32
	reentrancy     ReentrancyMode
	idleTimeout    time.Duration
	lastActivity   time.Time
}

func NewActor(id string, aType ActorType, maxConcurrency int32, reentrancy ReentrancyMode, idleTimeout time.Duration, now time.Time) *Actor {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	return &Actor{
		id: id, actorType: aType, maxConcurrency: maxConcurrency,
		reentrancy: reentrancy, idleTimeout: idleTimeout, lastActivity: now,
		vector: ActorStateVector{Lifecycle: ActorInactive, Residency: ResidencyCold, Concurrency: ConcurrencyIdle},
	}
}

func (a *Actor) ID() string                    { a.mu.RLock(); defer a.mu.RUnlock(); return a.id }
func (a *Actor) StateVector() ActorStateVector { a.mu.RLock(); defer a.mu.RUnlock(); return a.vector }

func (a *Actor) Activate(now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vector.Lifecycle == ActorActive {
		return nil
	}
	a.vector.Lifecycle, a.lastActivity = ActorActive, now
	return nil
}

func (a *Actor) Deactivate(now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeSlots > 0 {
		return fmt.Errorf("%w: cannot deactivate actor with %d active slots", ErrInvalidTransition, a.activeSlots)
	}
	a.vector.Lifecycle, a.vector.Concurrency, a.lastActivity = ActorInactive, ConcurrencyIdle, now
	return nil
}

func (a *Actor) SetResidency(residency ActorResidency) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.vector.Residency = residency
	return nil
}

func (a *Actor) AcquireSlot(now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vector.Lifecycle != ActorActive {
		return fmt.Errorf("%w: actor lifecycle is %s", ErrActorNotActive, a.vector.Lifecycle)
	}
	if a.vector.Residency != ResidencyResident {
		return fmt.Errorf("%w: actor residency is %s", ErrActorNotResident, a.vector.Residency)
	}
	if a.activeSlots > 0 && a.reentrancy == ReentrancyForbidden {
		return fmt.Errorf("%w: active slots %d", ErrReentrancyProhibited, a.activeSlots)
	}
	if a.activeSlots >= a.maxConcurrency {
		return fmt.Errorf("%w: active slots %d at max %d", ErrCapacityExceeded, a.activeSlots, a.maxConcurrency)
	}
	a.activeSlots++
	a.lastActivity = now
	if a.activeSlots >= a.maxConcurrency {
		a.vector.Concurrency = ConcurrencyAtCapacity
	} else {
		a.vector.Concurrency = ConcurrencyProcessing
	}
	return nil
}

func (a *Actor) ReleaseSlot(now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeSlots <= 0 {
		return fmt.Errorf("%w: cannot release from zero active slots", ErrInvalidTransition)
	}
	a.activeSlots--
	a.lastActivity = now
	if a.activeSlots == 0 {
		a.vector.Concurrency = ConcurrencyIdle
	} else {
		a.vector.Concurrency = ConcurrencyProcessing
	}
	return nil
}

func (a *Actor) CheckIdle(now time.Time) (IdleStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeSlots > 0 {
		return IdleStatusActive, nil
	}
	if a.idleTimeout > 0 && now.Sub(a.lastActivity) >= a.idleTimeout {
		a.vector.Lifecycle, a.vector.Concurrency = ActorInactive, ConcurrencyIdle
		return IdleStatusTimedOut, nil
	}
	return IdleStatusActive, nil
}

type SessionStateVector struct {
	Phase    SessionPhase
	Cache    SessionCacheState
	Affinity SessionAffinityState
}

type InferenceSession struct {
	mu            sync.RWMutex
	id            string
	modelRef      string
	actorRef      string
	groupRef      string
	boundEndpoint string
	vector        SessionStateVector
	ttl           time.Duration
	maxTurns      int32
	turnCount     int32
	tokenCount    int64
	kvCacheSize   int64
	lastActivity  time.Time
}

func NewInferenceSession(id, modelRef, actorRef, groupRef string, ttl time.Duration, maxTurns int32, now time.Time) *InferenceSession {
	return &InferenceSession{
		id: id, modelRef: modelRef, actorRef: actorRef, groupRef: groupRef,
		ttl: ttl, maxTurns: maxTurns, lastActivity: now,
		vector: SessionStateVector{Phase: SessionPhaseActive, Cache: CacheEmpty, Affinity: AffinityUnbound},
	}
}

func (s *InferenceSession) ID() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.id }
func (s *InferenceSession) StateVector() SessionStateVector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vector
}
func (s *InferenceSession) BoundEndpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.boundEndpoint
}
func (s *InferenceSession) TurnCount() int32 { s.mu.RLock(); defer s.mu.RUnlock(); return s.turnCount }

func (s *InferenceSession) BindEndpoint(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vector.Phase == SessionPhaseEvicted || s.vector.Phase == SessionPhaseCompleted {
		return fmt.Errorf("%w: cannot bind terminal session", ErrSessionTerminal)
	}
	s.boundEndpoint, s.vector.Affinity = endpoint, AffinityBound
	return nil
}

func (s *InferenceSession) RecordTurn(tokens int64, cacheBytes int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vector.Phase == SessionPhaseEvicted || s.vector.Phase == SessionPhaseCompleted {
		return fmt.Errorf("%w: session phase %s", ErrSessionTerminal, s.vector.Phase)
	}
	s.turnCount++
	s.tokenCount += tokens
	s.kvCacheSize, s.lastActivity, s.vector.Cache = cacheBytes, now, CacheResident
	if s.maxTurns > 0 && s.turnCount >= s.maxTurns {
		s.vector.Phase, s.vector.Affinity, s.vector.Cache = SessionPhaseCompleted, AffinityUnbound, CacheEvicted
		s.boundEndpoint = ""
		return ErrMaxTurnsReached
	}
	return nil
}

func (s *InferenceSession) CheckExpiry(now time.Time) SessionExpiryStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vector.Phase == SessionPhaseEvicted || s.vector.Phase == SessionPhaseCompleted {
		return SessionExpired
	}
	if s.ttl > 0 && now.Sub(s.lastActivity) >= s.ttl {
		s.vector.Phase, s.vector.Cache, s.vector.Affinity = SessionPhaseEvicted, CacheEvicted, AffinityUnbound
		s.boundEndpoint = ""
		return SessionExpired
	}
	return SessionLive
}

func (s *InferenceSession) SetCacheState(state SessionCacheState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vector.Cache = state
}

type GroupStateVector struct {
	Phase    CoactorGroupPhase
	Pattern  CoactorPattern
	Transfer TransferPhase
}

type TransferFence struct {
	Epoch          uint64
	SessionID      string
	PrefillActorID string
	DecodeActorID  string
	PrefixTokens   int64
	Digest         string
	Mechanism      TransferMechanism
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type CoactorMember struct {
	Name      string
	Role      MemberRole
	Actor     *Actor
	DependsOn []string
}

type CoActorGroup struct {
	mu           sync.RWMutex
	id           string
	pattern      CoactorPattern
	members      map[string]*CoactorMember
	vector       GroupStateVector
	currentEpoch uint64
	activeFence  *TransferFence
}

func NewCoActorGroup(id string, pattern CoactorPattern) *CoActorGroup {
	return &CoActorGroup{
		id: id, pattern: pattern, members: make(map[string]*CoactorMember),
		vector: GroupStateVector{Phase: GroupPhaseForming, Pattern: pattern, Transfer: TransferUninitialized},
	}
}

func (g *CoActorGroup) StateVector() GroupStateVector {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.vector
}

func (g *CoActorGroup) AddMember(name string, role MemberRole, actor *Actor, dependsOn []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.members[name] = &CoactorMember{Name: name, Role: role, Actor: actor, DependsOn: dependsOn}
}

func (g *CoActorGroup) RefreshPhase() CoactorGroupPhase {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.members) == 0 {
		g.vector.Phase = GroupPhaseDissolved
		return g.vector.Phase
	}
	readyCount := 0
	for _, m := range g.members {
		sv := m.Actor.StateVector()
		if sv.Lifecycle == ActorActive && sv.Residency == ResidencyResident {
			readyCount++
		}
	}
	if readyCount == len(g.members) {
		g.vector.Phase = GroupPhaseReady
	} else if readyCount > 0 {
		g.vector.Phase = GroupPhaseDegraded
	} else {
		g.vector.Phase = GroupPhaseForming
	}
	return g.vector.Phase
}

func (g *CoActorGroup) findMemberByRole(role MemberRole) (*CoactorMember, error) {
	for _, m := range g.members {
		if m.Role == role {
			return m, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrMemberNotFound, role)
}

func (g *CoActorGroup) InitiatePrefill(session *InferenceSession) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pattern != PatternPrefillDecode {
		return fmt.Errorf("%w: initiate prefill requires %s", ErrInvalidPattern, PatternPrefillDecode)
	}
	if g.vector.Phase != GroupPhaseReady {
		return fmt.Errorf("%w: group phase %s is not ready", ErrInvalidTransition, g.vector.Phase)
	}
	prefillMember, err := g.findMemberByRole(RolePrefill)
	if err != nil {
		return err
	}
	if err := prefillMember.Actor.AcquireSlot(time.Now()); err != nil {
		return fmt.Errorf("prefill slot acquire failed: %w", err)
	}
	g.vector.Transfer = TransferPrefilling
	session.SetCacheState(CacheWarming)
	return nil
}

func (g *CoActorGroup) CreateTransferFence(session *InferenceSession, tokens int64, digest string, ttl time.Duration, now time.Time) (*TransferFence, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pattern != PatternPrefillDecode {
		return nil, fmt.Errorf("%w: transfer fence requires %s", ErrInvalidPattern, PatternPrefillDecode)
	}
	prefill, err := g.findMemberByRole(RolePrefill)
	if err != nil {
		return nil, err
	}
	decode, err := g.findMemberByRole(RoleDecode)
	if err != nil {
		return nil, err
	}
	g.currentEpoch++
	fence := &TransferFence{
		Epoch:          g.currentEpoch,
		SessionID:      session.ID(),
		PrefillActorID: prefill.Actor.ID(),
		DecodeActorID:  decode.Actor.ID(),
		PrefixTokens:   tokens,
		Digest:         digest,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
	}
	g.activeFence = fence
	g.vector.Transfer = TransferFenced
	session.SetCacheState(CacheFenced)
	return fence, nil
}

func (g *CoActorGroup) BeginTransfer(fence *TransferFence, method TransferMechanism) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fence == nil || fence.Epoch != g.currentEpoch {
		return fmt.Errorf("%w: epoch mismatch", ErrStaleFence)
	}
	if g.vector.Transfer != TransferFenced {
		return fmt.Errorf("%w: current transfer phase %s", ErrInvalidTransition, g.vector.Transfer)
	}
	fence.Mechanism = method
	g.vector.Transfer = TransferTransmitting
	return nil
}

func (g *CoActorGroup) VerifyAndCompleteHandoff(fence *TransferFence, receivedDigest string, decodeEndpoint string, now time.Time, session *InferenceSession) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fence == nil || fence.Epoch != g.currentEpoch {
		return fmt.Errorf("%w: invalid fence", ErrStaleFence)
	}
	if now.After(fence.ExpiresAt) {
		g.vector.Transfer = TransferFailed
		return fmt.Errorf("%w: fence expired at %s", ErrFenceExpired, fence.ExpiresAt)
	}
	if fence.Digest != receivedDigest {
		g.vector.Transfer = TransferFailed
		session.SetCacheState(CacheEmpty)
		return fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, fence.Digest, receivedDigest)
	}
	decodeMember, err := g.findMemberByRole(RoleDecode)
	if err != nil {
		return err
	}
	if err := decodeMember.Actor.AcquireSlot(now); err != nil {
		g.vector.Transfer = TransferFailed
		return fmt.Errorf("decode slot acquire failed: %w", err)
	}
	if prefillMember, pErr := g.findMemberByRole(RolePrefill); pErr == nil {
		_ = prefillMember.Actor.ReleaseSlot(now)
	}
	g.vector.Transfer = TransferCompleted
	session.SetCacheState(CacheTransferred)
	if err := session.BindEndpoint(decodeEndpoint); err != nil {
		return fmt.Errorf("failed binding decode endpoint: %w", err)
	}
	return nil
}

func (g *CoActorGroup) AbortTransfer(fence *TransferFence, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fence == nil || fence.Epoch != g.currentEpoch {
		return fmt.Errorf("%w: invalid epoch during abort", ErrStaleFence)
	}
	g.vector.Transfer = TransferAborted
	g.vector.Phase = GroupPhaseDegraded
	return nil
}
