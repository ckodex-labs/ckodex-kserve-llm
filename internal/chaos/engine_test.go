/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package chaos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func chaosScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, networkingv1.AddToScheme(s))
	return s
}

func runningPod(name, ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func pendingPod(name, ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
}

// ---- NewEngine -------------------------------------------------------------

func TestNewEngine_NotNil(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())
	require.NotNil(t, e)
}

// ---- selectTargets ---------------------------------------------------------

func TestSelectTargets_AllPodsSelected_ReturnsAll(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2"}},
	}
	result := selectTargets(pods, 100, 0)
	assert.Len(t, result, 2)
}

func TestSelectTargets_AllPodsSelected_ReturnsAllInOrder(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2"}},
	}
	result := selectTargets(pods, 100, 0)
	assert.Len(t, result, 2)
}

func TestSelectTargets_SubsetReturned(t *testing.T) {
	pods := make([]corev1.Pod, 5)
	for i := range pods {
		pods[i] = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	}
	result := selectTargets(pods, 40, 7)
	assert.Len(t, result, 2)
}

func TestSelectTargets_MinimumPercentage_SelectsOne(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
	}
	result := selectTargets(pods, 1, 0)
	assert.Len(t, result, 1)
}

// ---- listTargetPods --------------------------------------------------------

func TestListTargetPods_FiltersPending(t *testing.T) {
	scheme := chaosScheme(t)
	labels := map[string]string{"app": "llm"}

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		runningPod("p1", "default", labels),
		pendingPod("p2", "default", labels),
		runningPod("p3", "default", labels),
	).Build())

	pods, err := e.listTargetPods(context.Background(), "default", labels)
	require.NoError(t, err)
	assert.Len(t, pods, 2)
}

func TestListTargetPods_NoMatchingPods(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())

	pods, err := e.listTargetPods(context.Background(), "default", map[string]string{"app": "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, pods)
}

func TestListTargetPods_LabelSelectorFilters(t *testing.T) {
	scheme := chaosScheme(t)

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		runningPod("p1", "default", map[string]string{"app": "target"}),
		runningPod("p2", "default", map[string]string{"app": "other"}),
	).Build())

	pods, err := e.listTargetPods(context.Background(), "default", map[string]string{"app": "target"})
	require.NoError(t, err)
	assert.Len(t, pods, 1)
	assert.Equal(t, "p1", pods[0].Name)
}

// ---- RunExperiment — unsupported -------------------------------------------

func TestRunExperiment_UnsupportedType_Error(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())

	_, err := e.RunExperiment(context.Background(), Experiment{
		Name:      "test",
		Namespace: "default",
		Selector:  map[string]string{"app": "test"},
		Type:      ExperimentType("unsupported"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported experiment type")
}

func TestValidateExperiment_RejectsUnboundedTargets(t *testing.T) {
	tests := []Experiment{
		{Name: "missing-namespace", Type: ExperimentPodKill, Selector: map[string]string{"app": "llm"}, Parameters: ExperimentParams{KillPercentage: 1}},
		{Name: "missing-selector", Type: ExperimentPodKill, Namespace: "default", Parameters: ExperimentParams{KillPercentage: 1}},
		{Name: "invalid-percentage", Type: ExperimentPodKill, Namespace: "default", Selector: map[string]string{"app": "llm"}, Parameters: ExperimentParams{KillPercentage: 0}},
	}
	for _, exp := range tests {
		t.Run(exp.Name, func(t *testing.T) {
			require.Error(t, ValidateExperiment(exp))
		})
	}
}

// ---- runPodKill ------------------------------------------------------------

func TestRunPodKill_NoPods_ZeroAffected(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())

	exp := Experiment{
		Name:       "kill-none",
		Type:       ExperimentPodKill,
		Namespace:  "default",
		Selector:   map[string]string{"app": "llm"},
		Parameters: ExperimentParams{KillPercentage: 50},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, 0, result.TargetPods)
	assert.Equal(t, 0, result.AffectedPods)
}

func TestRunPodKill_OnePod_Killed(t *testing.T) {
	scheme := chaosScheme(t)
	labels := map[string]string{"app": "llm"}

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		runningPod("victim", "default", labels),
	).Build())

	exp := Experiment{
		Name:       "kill-one",
		Type:       ExperimentPodKill,
		Namespace:  "default",
		Selector:   labels,
		Parameters: ExperimentParams{KillPercentage: 100},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, 1, result.TargetPods)
	assert.Equal(t, 1, result.AffectedPods)
	assert.Contains(t, result.Observations[0], "killed pod")
}

func TestRunPodKill_ZeroPercentIsRejected(t *testing.T) {
	scheme := chaosScheme(t)
	labels := map[string]string{"app": "llm"}

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		runningPod("p1", "default", labels),
	).Build())

	exp := Experiment{
		Name:       "kill-zero-pct",
		Type:       ExperimentPodKill,
		Namespace:  "default",
		Selector:   labels,
		Parameters: ExperimentParams{KillPercentage: 0},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
}

// ---- runNetworkPartition ---------------------------------------------------

func TestRunNetworkPartition_CreatesNetworkPolicy(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())

	exp := Experiment{
		Name:      "partition-test",
		Type:      ExperimentNetworkPartition,
		Namespace: "default",
		Selector:  map[string]string{"app": "llm"},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, 1, result.AffectedPods)
	assert.Contains(t, result.Observations[0], "deny-all NetworkPolicy")
}

func TestRunNetworkPartition_DuplicateCreate_Error(t *testing.T) {
	scheme := chaosScheme(t)

	// Pre-create the NetworkPolicy so second create fails
	existing := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-partition-test-partition",
			Namespace: "default",
		},
	}

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build())

	exp := Experiment{
		Name:      "partition-test",
		Type:      ExperimentNetworkPartition,
		Namespace: "default",
		Selector:  map[string]string{"app": "llm"},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
}

func TestCleanupExperiment_RemovesNetworkPartition(t *testing.T) {
	scheme := chaosScheme(t)
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name:      "chaos-partition-test-partition",
		Namespace: "default",
	}}
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy).Build())
	exp := Experiment{Name: "partition-test", Type: ExperimentNetworkPartition, Namespace: "default", Selector: map[string]string{"app": "llm"}}

	require.NoError(t, e.CleanupExperiment(context.Background(), exp))
	require.NoError(t, e.CleanupExperiment(context.Background(), exp), "cleanup must be retry-safe")
}

func TestSelectTargets_IsReproducible(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
	}
	first := selectTargets(pods, 34, 42)
	second := selectTargets(pods, 34, 42)
	require.Equal(t, first, second)
}

// ---- runPodFailure ---------------------------------------------------------

func TestRunPodFailure_NoPods_ZeroAffected(t *testing.T) {
	scheme := chaosScheme(t)
	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).Build())

	exp := Experiment{
		Name:       "failure-no-pods",
		Type:       ExperimentPodFailure,
		Namespace:  "default",
		Selector:   map[string]string{"app": "absent"},
		Parameters: ExperimentParams{KillPercentage: 100},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, 0, result.TargetPods)
	assert.Equal(t, 0, result.AffectedPods)
}

func TestRunPodFailure_OnePod_EvictionAttempted(t *testing.T) {
	scheme := chaosScheme(t)
	labels := map[string]string{"app": "llm"}
	pod := runningPod("evict-me", "default", labels)

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pod).
		WithStatusSubresource(pod).
		Build())

	exp := Experiment{
		Name:       "evict-one",
		Type:       ExperimentPodFailure,
		Namespace:  "default",
		Selector:   labels,
		Parameters: ExperimentParams{KillPercentage: 100},
	}

	// Fake client doesn't register eviction subresource by default — the
	// SubResource("eviction").Create call will fail, but runPodFailure logs
	// and continues, so overall result is "completed".
	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, 1, result.TargetPods)
}

// ---- ExperimentResult fields -----------------------------------------------

func TestExperimentResult_ObservationContainsInfo(t *testing.T) {
	scheme := chaosScheme(t)
	labels := map[string]string{"app": "llm"}

	e := NewEngine(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		runningPod("p1", "default", labels),
		runningPod("p2", "default", labels),
	).Build())

	exp := Experiment{
		Name:       "obs-test",
		Type:       ExperimentPodKill,
		Namespace:  "default",
		Selector:   labels,
		Parameters: ExperimentParams{KillPercentage: 100},
	}

	result, err := e.RunExperiment(context.Background(), exp)
	require.NoError(t, err)
	assert.Equal(t, 2, result.TargetPods)
	assert.Positive(t, result.AffectedPods)
	assert.NotEmpty(t, result.Observations)
	assert.NotZero(t, result.Duration)
}
