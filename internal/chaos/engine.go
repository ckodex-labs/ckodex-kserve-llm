/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package chaos provides a native chaos engine for fault injection testing.
package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ExperimentType defines the type of chaos experiment.
type ExperimentType string

const (
	ExperimentPodKill          ExperimentType = "pod-kill"
	ExperimentPodFailure       ExperimentType = "pod-failure"
	ExperimentNetworkPartition ExperimentType = "network-partition"
)

// Experiment defines a chaos experiment specification.
type Experiment struct {
	// Name of the experiment.
	Name string
	// Type of chaos to inject.
	Type ExperimentType
	// Namespace to target.
	Namespace string
	// Selector for target pods.
	Selector map[string]string
	// Duration of the experiment.
	Duration time.Duration
	// Seed selects targets reproducibly. A zero seed is deterministic.
	Seed int64
	// Parameters are experiment-specific settings.
	Parameters ExperimentParams
}

// ExperimentParams holds experiment-specific configuration.
type ExperimentParams struct {
	// PodKill: percentage of matching pods to kill.
	KillPercentage int `json:"killPercentage,omitempty"`
}

// ExperimentResult records the outcome of a chaos experiment.
type ExperimentResult struct {
	Name         string         `json:"name"`
	Type         ExperimentType `json:"type"`
	Status       string         `json:"status"` // running, completed, failed
	StartTime    time.Time      `json:"startTime"`
	EndTime      time.Time      `json:"endTime"`
	Duration     time.Duration  `json:"duration"`
	TargetPods   int            `json:"targetPods"`
	AffectedPods int            `json:"affectedPods"`
	Observations []string       `json:"observations"`
	Error        string         `json:"error,omitempty"`
}

// Engine is the native chaos engine for fault injection testing.
type Engine struct {
	client.Client
}

// NewEngine creates a new chaos engine.
func NewEngine(c client.Client) *Engine {
	return &Engine{Client: c}
}

// RunExperiment executes a chaos experiment.
func (e *Engine) RunExperiment(ctx context.Context, exp Experiment) (*ExperimentResult, error) {
	if err := ValidateExperiment(exp); err != nil {
		return failedResult(exp, err)
	}

	logger := log.FromContext(ctx).WithValues("experiment", exp.Name, "type", exp.Type)
	logger.Info("starting chaos experiment")

	result := &ExperimentResult{
		Name:      exp.Name,
		Type:      exp.Type,
		Status:    "running",
		StartTime: time.Now(),
	}

	switch exp.Type {
	case ExperimentPodKill:
		return e.runPodKill(ctx, exp, result)
	case ExperimentPodFailure:
		return e.runPodFailure(ctx, exp, result)
	case ExperimentNetworkPartition:
		return e.runNetworkPartition(ctx, exp, result)
	default:
		return fail(result, fmt.Errorf("unsupported experiment type: %s", exp.Type))
	}
}

// ValidateExperiment rejects unbounded or ambiguous destructive experiments.
func ValidateExperiment(exp Experiment) error {
	if strings.TrimSpace(exp.Name) == "" {
		return fmt.Errorf("experiment name is required")
	}
	if strings.TrimSpace(exp.Namespace) == "" {
		return fmt.Errorf("experiment namespace is required")
	}
	if len(exp.Selector) == 0 {
		return fmt.Errorf("experiment selector is required")
	}
	if exp.Type == ExperimentPodKill || exp.Type == ExperimentPodFailure {
		if exp.Parameters.KillPercentage < 1 || exp.Parameters.KillPercentage > 100 {
			return fmt.Errorf("kill percentage must be between 1 and 100")
		}
	}
	return nil
}

// CleanupExperiment removes resources created by an experiment. It is safe to retry.
func (e *Engine) CleanupExperiment(ctx context.Context, exp Experiment) error {
	if exp.Type != ExperimentNetworkPartition {
		return nil
	}
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("chaos-%s-partition", exp.Name),
		Namespace: exp.Namespace,
	}}
	if err := e.Delete(ctx, policy); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete network partition policy: %w", err)
	}
	return nil
}

// runPodKill kills a percentage of matching pods.
func (e *Engine) runPodKill(ctx context.Context, exp Experiment, result *ExperimentResult) (*ExperimentResult, error) {
	logger := log.FromContext(ctx)

	pods, err := e.listTargetPods(ctx, exp.Namespace, exp.Selector)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}

	result.TargetPods = len(pods)
	selected := selectTargets(pods, exp.Parameters.KillPercentage, exp.Seed)
	for _, pod := range selected {
		logger.Info("killing pod", "pod", pod.Name)
		if err := e.Delete(ctx, &pod); err != nil {
			return fail(result, fmt.Errorf("delete pod %s: %w", pod.Name, err))
		} else {
			result.AffectedPods++
			result.Observations = append(result.Observations,
				fmt.Sprintf("killed pod %s", pod.Name))
		}
	}

	return complete(result)
}

// runPodFailure evicts pods via the Eviction API.
func (e *Engine) runPodFailure(ctx context.Context, exp Experiment, result *ExperimentResult) (*ExperimentResult, error) {
	logger := log.FromContext(ctx)
	pods, err := e.listTargetPods(ctx, exp.Namespace, exp.Selector)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}

	result.TargetPods = len(pods)
	selected := selectTargets(pods, exp.Parameters.KillPercentage, exp.Seed)
	for _, pod := range selected {
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := e.SubResource("eviction").Create(ctx, &pod, eviction); err != nil {
			logger.Error(err, "eviction failed", "pod", pod.Name)
			return fail(result, fmt.Errorf("evict pod %s: %w", pod.Name, err))
		} else {
			result.AffectedPods++
			result.Observations = append(result.Observations,
				fmt.Sprintf("evicted pod %s", pod.Name))
		}
	}

	return complete(result)
}

// runNetworkPartition creates a deny-all NetworkPolicy to simulate partition.
func (e *Engine) runNetworkPartition(ctx context.Context, exp Experiment, result *ExperimentResult) (*ExperimentResult, error) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("chaos-%s-partition", exp.Name),
			Namespace: exp.Namespace,
			Labels: map[string]string{
				"chaos.ckodex.org/experiment": exp.Name,
				"chaos.ckodex.org/type":       "network-partition",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: exp.Selector},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// Empty ingress+egress = deny all
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress:  []networkingv1.NetworkPolicyEgressRule{},
		},
	}

	if err := e.Create(ctx, policy); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}

	result.AffectedPods = 1 // policy-level
	result.Observations = append(result.Observations,
		fmt.Sprintf("created deny-all NetworkPolicy %s for partition", policy.Name))
	return complete(result)
}

func (e *Engine) listTargetPods(ctx context.Context, namespace string, selector map[string]string) ([]corev1.Pod, error) {
	var podList corev1.PodList
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(selector),
	}
	if err := e.List(ctx, &podList, opts...); err != nil {
		return nil, fmt.Errorf("list target pods: %w", err)
	}

	// Filter to running pods only
	var running []corev1.Pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			running = append(running, pod)
		}
	}
	return running, nil
}

func selectTargets(pods []corev1.Pod, percentage int, seed int64) []corev1.Pod {
	count := len(pods) * percentage / 100
	if count == 0 && len(pods) > 0 {
		count = 1
	}
	if count >= len(pods) {
		return pods
	}
	shuffled := append([]corev1.Pod(nil), pods...)
	sort.Slice(shuffled, func(i, j int) bool { return shuffled[i].Name < shuffled[j].Name })
	rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled[:count]
}

func complete(result *ExperimentResult) (*ExperimentResult, error) {
	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

func fail(result *ExperimentResult, err error) (*ExperimentResult, error) {
	result.Status = "failed"
	result.Error = err.Error()
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, err
}

func failedResult(exp Experiment, err error) (*ExperimentResult, error) {
	result := &ExperimentResult{Name: exp.Name, Type: exp.Type, StartTime: time.Now()}
	return fail(result, err)
}
