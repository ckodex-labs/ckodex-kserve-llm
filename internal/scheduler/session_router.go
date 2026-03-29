/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// SessionRouter makes session-aware routing decisions for EPP.
// When a request carries a session ID, routes to the bound endpoint
// for KV-cache reuse (prefix-cache hit).
type SessionRouter struct {
	client.Client
}

// RouteDecision is the result of a routing decision.
type RouteDecision struct {
	// Endpoint is the target pod IP:port.
	Endpoint string
	// SessionID is the session that influenced this decision.
	SessionID string
	// Reason explains the routing decision.
	Reason string
	// CacheHit indicates whether this is a prefix-cache hit.
	CacheHit bool
}

// Route determines the target endpoint for a request.
// Priority: 1. Session-bound endpoint (prefix-cache hit)
//  2. EPP selection (KV-cache aware)
//  3. Round-robin fallback
func (r *SessionRouter) Route(ctx context.Context, modelRef, sessionID, namespace string) (*RouteDecision, error) {
	logger := log.FromContext(ctx).WithValues("component", "session-router")

	// 1. Check for session-bound endpoint
	if sessionID != "" {
		decision, err := r.routeBySession(ctx, sessionID, namespace)
		if err == nil && decision != nil {
			logger.Info("session-affinity routing",
				"session", sessionID,
				"endpoint", decision.Endpoint,
			)
			return decision, nil
		}
		// Session not found or endpoint invalid — fall through to EPP
	}

	// 2. Fall through to EPP-based routing
	return &RouteDecision{
		Reason: "no-session-affinity-epp-routing",
	}, nil
}

// routeBySession looks up the session's bound endpoint.
func (r *SessionRouter) routeBySession(ctx context.Context, sessionID, namespace string) (*RouteDecision, error) {
	var session servingv1alpha2.InferenceSession
	key := client.ObjectKey{Name: sessionID, Namespace: namespace}
	if err := r.Get(ctx, key, &session); err != nil {
		return nil, err
	}

	// Only route to active sessions with a bound endpoint
	if session.Status.Phase != servingv1alpha2.SessionPhaseActive ||
		session.Status.BoundEndpoint == "" {
		return nil, nil
	}

	return &RouteDecision{
		Endpoint:  session.Status.BoundEndpoint,
		SessionID: sessionID,
		Reason:    "session-affinity-kv-cache-hit",
		CacheHit:  true,
	}, nil
}

// RecordActivity updates the session's last activity time and turn count.
func (r *SessionRouter) RecordActivity(ctx context.Context, sessionID, namespace string, tokenCount int64) error {
	var session servingv1alpha2.InferenceSession
	key := client.ObjectKey{Name: sessionID, Namespace: namespace}
	if err := r.Get(ctx, key, &session); err != nil {
		return err
	}

	session.Status.TurnCount++
	session.Status.TokenCount += tokenCount
	now := metav1.Now()
	session.Status.LastActivityTime = &now
	session.Status.Phase = servingv1alpha2.SessionPhaseActive

	return r.Status().Update(ctx, &session)
}
