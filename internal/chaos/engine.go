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
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
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
	ExperimentNetworkLatency   ExperimentType = "network-latency"
	ExperimentCPUStress        ExperimentType = "cpu-stress"
	ExperimentMemoryStress     ExperimentType = "memory-stress"
	ExperimentDiskPressure     ExperimentType = "disk-pressure"
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
	// Parameters are experiment-specific settings.
	Parameters ExperimentParams
}

// ExperimentParams holds experiment-specific configuration.
type ExperimentParams struct {
	// PodKill: percentage of matching pods to kill.
	KillPercentage int `json:"killPercentage,omitempty"`
	// NetworkLatency: delay to inject in milliseconds.
	LatencyMs int `json:"latencyMs,omitempty"`
	// NetworkLatency: jitter in milliseconds.
	JitterMs int `json:"jitterMs,omitempty"`
	// CPUStress: number of workers.
	CPUWorkers int `json:"cpuWorkers,omitempty"`
	// MemoryStress: bytes to allocate.
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
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
	case ExperimentNetworkLatency:
		return e.runNetworkLatency(ctx, exp, result)
	case ExperimentNetworkPartition:
		return e.runNetworkPartition(ctx, exp, result)
	default:
		return nil, fmt.Errorf("unsupported experiment type: %s", exp.Type)
	}
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
	killCount := len(pods) * exp.Parameters.KillPercentage / 100
	if killCount < 1 && len(pods) > 0 {
		killCount = 1
	}

	// Select random pods to kill
	selected := selectRandom(pods, killCount)
	for _, pod := range selected {
		logger.Info("killing pod", "pod", pod.Name)
		if err := e.Delete(ctx, &pod); err != nil {
			result.Observations = append(result.Observations,
				fmt.Sprintf("failed to kill pod %s: %v", pod.Name, err))
		} else {
			result.AffectedPods++
			result.Observations = append(result.Observations,
				fmt.Sprintf("killed pod %s", pod.Name))
		}
	}

	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
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
	killCount := len(pods) * exp.Parameters.KillPercentage / 100
	if killCount < 1 && len(pods) > 0 {
		killCount = 1
	}

	selected := selectRandom(pods, killCount)
	for _, pod := range selected {
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := e.SubResource("eviction").Create(ctx, &pod, eviction); err != nil {
			logger.Info("eviction failed", "pod", pod.Name, "error", err)
			result.Observations = append(result.Observations,
				fmt.Sprintf("eviction failed for %s: %v", pod.Name, err))
		} else {
			result.AffectedPods++
			result.Observations = append(result.Observations,
				fmt.Sprintf("evicted pod %s", pod.Name))
		}
	}

	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

// runNetworkLatency annotates pods with latency injection metadata.
// Requires a CNI plugin (e.g., Cilium, Istio) that reads these annotations.
func (e *Engine) runNetworkLatency(ctx context.Context, exp Experiment, result *ExperimentResult) (*ExperimentResult, error) {
	logger := log.FromContext(ctx)
	pods, err := e.listTargetPods(ctx, exp.Namespace, exp.Selector)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}

	result.TargetPods = len(pods)
	for _, pod := range pods {
		patch := client.MergeFrom(pod.DeepCopy())
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations["chaos.ckodex.io/network-latency-ms"] = fmt.Sprintf("%d", exp.Parameters.LatencyMs)
		pod.Annotations["chaos.ckodex.io/network-jitter-ms"] = fmt.Sprintf("%d", exp.Parameters.JitterMs)
		pod.Annotations["chaos.ckodex.io/experiment"] = exp.Name
		if err := e.Patch(ctx, &pod, patch); err != nil {
			logger.Info("failed to annotate pod for latency", "pod", pod.Name, "error", err)
		} else {
			result.AffectedPods++
			result.Observations = append(result.Observations,
				fmt.Sprintf("injected %dms latency on %s", exp.Parameters.LatencyMs, pod.Name))
		}
	}

	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

// runNetworkPartition creates a deny-all NetworkPolicy to simulate partition.
func (e *Engine) runNetworkPartition(ctx context.Context, exp Experiment, result *ExperimentResult) (*ExperimentResult, error) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("chaos-%s-partition", exp.Name),
			Namespace: exp.Namespace,
			Labels: map[string]string{
				"chaos.ckodex.io/experiment": exp.Name,
				"chaos.ckodex.io/type":       "network-partition",
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
	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
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

func selectRandom(pods []corev1.Pod, count int) []corev1.Pod {
	if count >= len(pods) {
		return pods
	}
	shuffled := make([]corev1.Pod, len(pods))
	copy(shuffled, pods)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled[:count]
}

func init() {
	_ = metav1.Now // keep import
}
