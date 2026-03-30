/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// SessionReconciler watches InferenceSession CRs and manages
// session affinity routing, TTL enforcement, and KV-cache lifecycle.
type SessionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.io,resources=inferencesessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.io,resources=inferencesessions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch

func (r *SessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("session", req.NamespacedName)

	var session servingv1alpha2.InferenceSession
	if err := r.Get(ctx, req.NamespacedName, &session); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1. Check TTL expiry
	if session.Status.Phase == servingv1alpha2.SessionPhaseIdle {
		if expired := r.isExpired(&session); expired {
			logger.Info("session expired, evicting KV-cache")
			session.Status.Phase = servingv1alpha2.SessionPhaseEvicted
			session.Status.KVCacheSize = 0
			if err := r.Status().Update(ctx, &session); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// 2. Check max turns
	if session.Spec.MaxTurns > 0 && session.Status.TurnCount >= session.Spec.MaxTurns {
		logger.Info("session reached max turns", "turns", session.Status.TurnCount)
		session.Status.Phase = servingv1alpha2.SessionPhaseCompleted
		if err := r.Status().Update(ctx, &session); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 3. Ensure bound endpoint is still valid
	if session.Status.BoundEndpoint != "" {
		if err := r.validateEndpoint(ctx, &session); err != nil {
			logger.Info("bound endpoint invalid, rebinding", "error", err)
			session.Status.BoundEndpoint = ""
			session.Status.Phase = servingv1alpha2.SessionPhaseActive
		}
	}

	// 4. Bind session to an endpoint if unbound
	if session.Status.BoundEndpoint == "" && session.Status.Phase != servingv1alpha2.SessionPhaseEvicted {
		endpoint, err := r.selectEndpoint(ctx, &session)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		session.Status.BoundEndpoint = endpoint
		session.Status.Phase = servingv1alpha2.SessionPhaseActive
		logger.Info("session bound to endpoint", "endpoint", endpoint)
	}

	// 5. Update status
	if err := r.Status().Update(ctx, &session); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue for TTL check
	requeueAfter := 30 * time.Second
	if session.Spec.TTL != nil {
		requeueAfter = session.Spec.TTL.Duration / 2
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// isExpired checks if the session TTL has elapsed since last activity.
func (r *SessionReconciler) isExpired(session *servingv1alpha2.InferenceSession) bool {
	if session.Spec.TTL == nil || session.Status.LastActivityTime == nil {
		return false
	}
	return time.Since(session.Status.LastActivityTime.Time) > session.Spec.TTL.Duration
}

// validateEndpoint checks if the bound endpoint pod is still running
// by verifying the EndpointSlice resource for the model service contains this address.
func (r *SessionReconciler) validateEndpoint(ctx context.Context, session *servingv1alpha2.InferenceSession) error {
	if session.Status.BoundEndpoint == "" {
		return fmt.Errorf("no bound endpoint")
	}

	// Extract host from endpoint (host:port or host.ns.svc:port)
	host := session.Status.BoundEndpoint
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}

	// Look up the EndpointSlices for the model service
	var slices discoveryv1.EndpointSliceList
	if err := r.List(ctx, &slices, client.InNamespace(session.Namespace), client.MatchingLabels{
		"kubernetes.io/service-name": session.Spec.ModelRef,
	}); err != nil {
		return fmt.Errorf("endpointslices for %s not found: %w", session.Spec.ModelRef, err)
	}

	if len(slices.Items) == 0 {
		return fmt.Errorf("no endpointslices found for service %s", session.Spec.ModelRef)
	}

	// Check if the bound address exists in any slice
	for _, slice := range slices.Items {
		for _, ep := range slice.Endpoints {
			// Ready must be nil or true
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			for _, addr := range ep.Addresses {
				if addr == host || fmt.Sprintf("%s.%s.svc.cluster.local", session.Spec.ModelRef, session.Namespace) == host {
					return nil // endpoint is valid
				}
			}
		}
	}

	return fmt.Errorf("endpoint %s no longer in service %s endpoints", session.Status.BoundEndpoint, session.Spec.ModelRef)
}

// selectEndpoint picks the best endpoint for a new session.
// Looks up EndpointSlices and selects the least-loaded (by active sessions).
func (r *SessionReconciler) selectEndpoint(ctx context.Context, session *servingv1alpha2.InferenceSession) (string, error) {
	// Verify model exists
	var llmSvc servingv1alpha2.LLMInferenceService
	if err := r.Get(ctx, types.NamespacedName{
		Name:      session.Spec.ModelRef,
		Namespace: session.Namespace,
	}, &llmSvc); err != nil {
		return "", fmt.Errorf("model %s not found: %w", session.Spec.ModelRef, err)
	}

	// Look up EndpointSlices to find ready pod IPs
	var slices discoveryv1.EndpointSliceList
	if err := r.List(ctx, &slices, client.InNamespace(session.Namespace), client.MatchingLabels{
		"kubernetes.io/service-name": session.Spec.ModelRef,
	}); err != nil || len(slices.Items) == 0 {
		// Fallback to service DNS
		return fmt.Sprintf("%s.%s.svc.cluster.local:8000",
			llmSvc.Name, llmSvc.Namespace), nil
	}

	// Count active sessions per endpoint to pick the least-loaded
	var sessionList servingv1alpha2.InferenceSessionList
	_ = r.List(ctx, &sessionList,
		client.InNamespace(session.Namespace),
		client.MatchingFields{"spec.modelRef": session.Spec.ModelRef},
	)

	loadMap := make(map[string]int)
	for _, s := range sessionList.Items {
		if s.Status.Phase == servingv1alpha2.SessionPhaseActive && s.Status.BoundEndpoint != "" {
			loadMap[s.Status.BoundEndpoint]++
		}
	}

	// Select endpoint with fewest active sessions
	bestEndpoint := ""
	bestLoad := int(^uint(0) >> 1) // max int
	for _, slice := range slices.Items {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			for _, addr := range ep.Addresses {
				epStr := fmt.Sprintf("%s:8000", addr)
				if load := loadMap[epStr]; load < bestLoad {
					bestLoad = load
					bestEndpoint = epStr
				}
			}
		}
	}

	if bestEndpoint == "" {
		return fmt.Sprintf("%s.%s.svc.cluster.local:8000",
			llmSvc.Name, llmSvc.Namespace), nil
	}

	return bestEndpoint, nil
}

func (r *SessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha2.InferenceSession{}).
		Complete(r)
}

// ActorReconciler watches InferenceActor CRs and manages
// Dapr actor registration and lifecycle.
type ActorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.io,resources=inferenceactors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.io,resources=inferenceactors/status,verbs=get;update;patch

func (r *ActorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("actor", req.NamespacedName)

	var actor servingv1alpha2.InferenceActor
	if err := r.Get(ctx, req.NamespacedName, &actor); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1. Check idle timeout for deactivation
	if actor.Status.State == servingv1alpha2.ActorStateActive &&
		actor.Status.ActiveSessions == 0 &&
		actor.Spec.IdleTimeout != nil &&
		actor.Status.LastActivationTime != nil {

		if time.Since(actor.Status.LastActivationTime.Time) > actor.Spec.IdleTimeout.Duration {
			logger.Info("deactivating idle actor")
			actor.Status.State = servingv1alpha2.ActorStateDeactivating
			if err := r.Status().Update(ctx, &actor); err != nil {
				return ctrl.Result{}, err
			}
			// Dapr actor deactivation: DELETE /actors/{actorType}/{actorId}
			// The Dapr sidecar handles deactivation when no reminders/timers remain.
			// We mark the actor as Inactive and let the sidecar GC handle the rest.
			logger.Info("actor deactivated", "actor", actor.Name, "type", actor.Spec.ActorType)
			actor.Status.State = servingv1alpha2.ActorStateInactive
			if err := r.Status().Update(ctx, &actor); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// 2. Activate actor if it has pending sessions
	if actor.Status.State == servingv1alpha2.ActorStateInactive &&
		actor.Status.ActiveSessions > 0 {
		logger.Info("activating actor", "actor", actor.Name, "sessions", actor.Status.ActiveSessions)
		now := metav1.Now()
		actor.Status.State = servingv1alpha2.ActorStateActive
		actor.Status.LastActivationTime = &now
		// Dapr actor activation: actors are lazily activated on first invocation.
		// The Dapr runtime activates the actor when it receives the first request
		// via POST /actors/{actorType}/{actorId}/method/{methodName}.
		// We simply mark the status as Active — the sidecar handles the rest.
	}

	if err := r.Status().Update(ctx, &actor); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ActorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha2.InferenceActor{}).
		Complete(r)
}

// CoactorGroupReconciler watches CoactorGroup CRs and manages
// collaborative actor groups.
type CoactorGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.ckodex.io,resources=coactorgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.ckodex.io,resources=coactorgroups/status,verbs=get;update;patch

func (r *CoactorGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("coactorgroup", req.NamespacedName)

	var group servingv1alpha2.CoactorGroup
	if err := r.Get(ctx, req.NamespacedName, &group); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1. Resolve member actors
	memberStatuses := make([]servingv1alpha2.MemberStatus, 0, len(group.Spec.Members))
	activeCount := int32(0)

	for _, member := range group.Spec.Members {
		var actor servingv1alpha2.InferenceActor
		err := r.Get(ctx, types.NamespacedName{
			Name: member.ActorRef, Namespace: group.Namespace,
		}, &actor)

		ms := servingv1alpha2.MemberStatus{
			Name:  member.Name,
			Role:  member.Role,
			State: servingv1alpha2.ActorStateInactive,
			Ready: false,
		}

		if err == nil {
			ms.State = actor.Status.State
			ms.Ready = actor.Status.State == servingv1alpha2.ActorStateActive
			if ms.Ready {
				activeCount++
			}
		}
		memberStatuses = append(memberStatuses, ms)
	}

	// 2. Determine group phase
	totalMembers := int32(len(group.Spec.Members))
	switch {
	case activeCount == totalMembers:
		group.Status.Phase = servingv1alpha2.CoactorGroupReady
	case activeCount > 0:
		group.Status.Phase = servingv1alpha2.CoactorGroupDegraded
	case activeCount == 0 && group.Status.Phase == servingv1alpha2.CoactorGroupReady:
		group.Status.Phase = servingv1alpha2.CoactorGroupDissolved
	default:
		group.Status.Phase = servingv1alpha2.CoactorGroupForming
	}

	group.Status.ActiveMemberCount = activeCount
	group.Status.MemberStatuses = memberStatuses

	logger.Info("coactor group reconciled",
		"pattern", group.Spec.Pattern,
		"active", activeCount,
		"total", totalMembers,
		"phase", group.Status.Phase,
	)

	if err := r.Status().Update(ctx, &group); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *CoactorGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha2.CoactorGroup{}).
		Complete(r)
}
