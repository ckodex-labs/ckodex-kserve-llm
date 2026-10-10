/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package kvstate

import (
	"fmt"
	"strings"
	"time"
)

// TransferPhase models P->D disaggregated KV-cache transfer progress.
type TransferPhase string

const (
	TransferPhaseIdle      TransferPhase = "idle"
	TransferPhaseInitiated TransferPhase = "initiated"
	TransferPhaseInFlight  TransferPhase = "in-flight"
	TransferPhaseCompleted TransferPhase = "completed"
	TransferPhaseFailed    TransferPhase = "failed"
	TransferPhaseCancelled TransferPhase = "cancelled"
)

// TransferRole specifies the node role in P->D disaggregation.
type TransferRole string

const (
	RoleProducer TransferRole = "kv_producer"
	RoleConsumer TransferRole = "kv_consumer"
	RoleBoth     TransferRole = "kv_both"
)

// TransferContract governs P->D KV-cache transfer between prefill and decode nodes.
type TransferContract struct {
	TransferID      string
	Identity        CacheIdentity
	SourceNode      string
	TargetNode      string
	Connector       string
	Role            TransferRole
	Phase           TransferPhase
	Tokens          int
	Bytes           int64
	PayloadChecksum string
	CreatedAt       time.Time
	Deadline        time.Time
	CompletedAt     time.Time
	FailureReason   string
}

// NewTransferContract instantiates a governed P->D transfer contract.
func NewTransferContract(
	id string, identity CacheIdentity, src, dst, conn string,
	role TransferRole, tokens int, bytesCount int64, deadline time.Time,
) (*TransferContract, error) {
	if err := identity.Validate(); err != nil {
		return nil, fmt.Errorf("transfer contract identity: %w", err)
	}
	if id == "" || src == "" || dst == "" || conn == "" {
		return nil, fmt.Errorf("%w: transfer contract requires non-empty ids and nodes", ErrTransferFailed)
	}
	if src == dst {
		return nil, fmt.Errorf("%w: source and destination cannot be identical", ErrTransferFailed)
	}
	return &TransferContract{
		TransferID: id, Identity: identity, SourceNode: src, TargetNode: dst,
		Connector: conn, Role: role, Phase: TransferPhaseIdle,
		Tokens: tokens, Bytes: bytesCount, Deadline: deadline,
	}, nil
}

// Initiate transitions the contract from Idle to Initiated.
func (t *TransferContract) Initiate(now time.Time) error {
	if t.Phase != TransferPhaseIdle {
		return fmt.Errorf("%w: cannot initiate from phase %s", ErrStateConflict, t.Phase)
	}
	t.CreatedAt = now
	t.Phase = TransferPhaseInitiated
	return nil
}

// MarkInFlight advances transfer phase to InFlight if deadline has not passed.
func (t *TransferContract) MarkInFlight(now time.Time) error {
	if !t.Deadline.IsZero() && !now.Before(t.Deadline) {
		t.Phase = TransferPhaseFailed
		t.FailureReason = "deadline exceeded prior to in-flight"
		return fmt.Errorf("%w: transfer deadline expired", ErrTransferTimeout)
	}
	if t.Phase != TransferPhaseInitiated {
		return fmt.Errorf("%w: cannot mark in-flight from phase %s", ErrStateConflict, t.Phase)
	}
	t.Phase = TransferPhaseInFlight
	return nil
}

// Complete finalizes transfer with integrity checksum verification.
func (t *TransferContract) Complete(now time.Time, checksum string) error {
	if !t.Deadline.IsZero() && !now.Before(t.Deadline) {
		t.Phase = TransferPhaseFailed
		t.FailureReason = "deadline exceeded before completion"
		return fmt.Errorf("%w: transfer deadline expired", ErrTransferTimeout)
	}
	if t.Phase != TransferPhaseInFlight {
		return fmt.Errorf("%w: cannot complete transfer in phase %s", ErrStateConflict, t.Phase)
	}
	if strings.TrimSpace(checksum) == "" {
		t.Phase = TransferPhaseFailed
		t.FailureReason = "missing payload checksum"
		return fmt.Errorf("%w: payload checksum is required", ErrIntegrityViolation)
	}
	t.PayloadChecksum = checksum
	t.CompletedAt = now
	t.Phase = TransferPhaseCompleted
	return nil
}

// Fail records an explicit transfer failure.
func (t *TransferContract) Fail(now time.Time, reason string) {
	t.Phase = TransferPhaseFailed
	t.FailureReason = reason
	t.CompletedAt = now
}

// VerifyGovernance ensures consumer adapter aligns with transferred KV state.
func (t *TransferContract) VerifyGovernance(consumerAdapter string) error {
	if t.Identity.AdapterID != "" && t.Identity.AdapterID != consumerAdapter {
		return fmt.Errorf("%w: expected adapter %q, consumer loaded %q",
			ErrAdapterMismatch, t.Identity.AdapterID, consumerAdapter)
	}
	return nil
}
