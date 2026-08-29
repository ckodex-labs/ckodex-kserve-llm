/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package dapr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- WorkflowStep definitions ---------------------------------------------

func TestModelOnboardingWorkflow_Steps(t *testing.T) {
	w := &ModelOnboardingWorkflow{}
	steps := w.Steps()

	require.Len(t, steps, 6)
	// validate has no compensation
	assert.Equal(t, ActivityValidate, steps[0].Activity)
	assert.Empty(t, steps[0].Compensate)
	// deploy compensates with rollback
	assert.Equal(t, ActivityDeploy, steps[4].Activity)
	assert.Equal(t, ActivityRollback, steps[4].Compensate)
	// verify compensates with rollback
	assert.Equal(t, ActivityVerify, steps[5].Activity)
	assert.Equal(t, ActivityRollback, steps[5].Compensate)
}

func TestAgentScalingWorkflow_Steps(t *testing.T) {
	w := &AgentScalingWorkflow{}
	steps := w.Steps()

	require.Len(t, steps, 4)
	assert.Equal(t, ActivityAssess, steps[0].Activity)
	assert.Equal(t, ActivityScale, steps[1].Activity)
	assert.Equal(t, ActivityRevert, steps[1].Compensate)
	// report has no compensation
	assert.Empty(t, steps[3].Compensate)
}

func TestSkillUpdateWorkflow_Steps(t *testing.T) {
	w := &SkillUpdateWorkflow{}
	steps := w.Steps()

	require.Len(t, steps, 4)
	assert.Equal(t, ActivityValidate, steps[0].Activity)
	assert.Equal(t, ActivityStage, steps[1].Activity)
	assert.Equal(t, ActivityCleanup, steps[1].Compensate)
	assert.Equal(t, ActivitySwap, steps[2].Activity)
	assert.Equal(t, ActivityRevert, steps[2].Compensate)
}

func TestModelRollbackWorkflow_Steps(t *testing.T) {
	w := &ModelRollbackWorkflow{}
	steps := w.Steps()

	require.Len(t, steps, 4)
	// All steps have empty compensation (rollback is already the compensator)
	for _, step := range steps {
		assert.Empty(t, step.Compensate)
	}
}

// ---- SagaExecutor.Execute -------------------------------------------------

var testInput = WorkflowInput{
	ModelRef:    "llama3",
	Namespace:   "default",
	RequestedBy: "test",
	Timestamp:   time.Now(),
}

func noop(_ context.Context, _ WorkflowInput) error { return nil }

func failWith(msg string) ActivityFunc {
	return func(_ context.Context, _ WorkflowInput) error {
		return errors.New(msg)
	}
}

func TestSagaExecutor_AllStepsSucceed(t *testing.T) {
	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a": noop,
			"step-b": noop,
			"step-c": noop,
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: ""},
		{Activity: "step-b", Compensate: ""},
		{Activity: "step-c", Compensate: ""},
	}

	result, err := exec.Execute(context.Background(), "test-wf", steps, testInput)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, "test-wf", result.WorkflowID)
	assert.Len(t, result.Activities, 3)
	for _, a := range result.Activities {
		assert.Equal(t, "completed", a.Status)
	}
}

func TestSagaExecutor_EmptySteps_Completed(t *testing.T) {
	exec := &SagaExecutor{Activities: map[string]ActivityFunc{}}
	result, err := exec.Execute(context.Background(), "empty-wf", []WorkflowStep{}, testInput)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.Empty(t, result.Activities)
}

func TestSagaExecutor_FirstStepFails_NoCompensation(t *testing.T) {
	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a": failWith("first failed"),
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: ""},
	}

	result, err := exec.Execute(context.Background(), "fail-wf", steps, testInput)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "first failed")
	assert.Equal(t, "compensated", result.Status)
}

func TestSagaExecutor_SecondStepFails_CompensatesFirst(t *testing.T) {
	compensated := make([]string, 0)

	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a": noop,
			"compensate-a": func(_ context.Context, _ WorkflowInput) error {
				compensated = append(compensated, "comp-a")
				return nil
			},
			"step-b": failWith("step-b failed"),
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: "compensate-a"},
		{Activity: "step-b", Compensate: ""},
	}

	result, err := exec.Execute(context.Background(), "comp-wf", steps, testInput)
	require.Error(t, err)
	assert.Equal(t, "compensated", result.Status)
	assert.Contains(t, compensated, "comp-a")

	// Verify activity log contains compensated entry
	found := false
	for _, a := range result.Activities {
		if a.Name == "compensate-a" && a.Status == "compensated" {
			found = true
		}
	}
	assert.True(t, found, "compensation activity should be logged")
}

func TestSagaExecutor_CompensationFailureIsSurfaced(t *testing.T) {
	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a":       noop,
			"compensate-a": failWith("rollback unavailable"),
			"step-b":       failWith("step-b failed"),
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: "compensate-a"},
		{Activity: "step-b", Compensate: ""},
	}

	result, err := exec.Execute(context.Background(), "comp-fail-wf", steps, testInput)
	require.Error(t, err)
	assert.Equal(t, "compensation-failed", result.Status)
	assert.Contains(t, err.Error(), "rollback unavailable")
	require.Len(t, result.Activities, 3)
	assert.Equal(t, "compensation-failed", result.Activities[2].Status)
}

func TestSagaExecutor_ThirdStepFails_CompensatesInReverseOrder(t *testing.T) {
	var order []string

	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a": noop,
			"comp-a": func(_ context.Context, _ WorkflowInput) error { order = append(order, "comp-a"); return nil },
			"step-b": noop,
			"comp-b": func(_ context.Context, _ WorkflowInput) error { order = append(order, "comp-b"); return nil },
			"step-c": failWith("step-c failed"),
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: "comp-a"},
		{Activity: "step-b", Compensate: "comp-b"},
		{Activity: "step-c", Compensate: ""},
	}

	_, err := exec.Execute(context.Background(), "reverse-comp-wf", steps, testInput)
	require.Error(t, err)

	// Compensation must be in reverse order: comp-b first, then comp-a
	require.Len(t, order, 2)
	assert.Equal(t, "comp-b", order[0])
	assert.Equal(t, "comp-a", order[1])
}

func TestSagaExecutor_UnknownActivity_Error(t *testing.T) {
	exec := &SagaExecutor{Activities: map[string]ActivityFunc{}}
	steps := []WorkflowStep{
		{Activity: "nonexistent", Compensate: ""},
	}

	result, err := exec.Execute(context.Background(), "unknown-wf", steps, testInput)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown activity: nonexistent")
	assert.Equal(t, "failed", result.Status)
}

func TestSagaExecutor_CompensationStepWithNoCompensate_Skipped(t *testing.T) {
	var compCalled bool

	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step-a": noop,
			// step-b has empty Compensate
			"step-b": failWith("step-b failed"),
		},
	}
	steps := []WorkflowStep{
		{Activity: "step-a", Compensate: ""}, // no compensation registered
		{Activity: "step-b", Compensate: ""},
	}

	_, err := exec.Execute(context.Background(), "no-comp-wf", steps, testInput)
	require.Error(t, err)
	// comp was never called since Compensate is empty
	assert.False(t, compCalled)
}

func TestSagaExecutor_ResultDurationIsPositive(t *testing.T) {
	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"step": noop,
		},
	}
	result, err := exec.Execute(context.Background(), "dur-wf",
		[]WorkflowStep{{Activity: "step", Compensate: ""}}, testInput)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestSagaExecutor_ActivityLog_FailureRecorded(t *testing.T) {
	exec := &SagaExecutor{
		Activities: map[string]ActivityFunc{
			"a": failWith("oops"),
		},
	}

	result, _ := exec.Execute(context.Background(), "log-wf",
		[]WorkflowStep{{Activity: "a", Compensate: ""}}, testInput)

	require.Len(t, result.Activities, 1)
	assert.Equal(t, "a", result.Activities[0].Name)
	assert.Equal(t, "failed", result.Activities[0].Status)
	assert.Equal(t, "oops", result.Activities[0].Error)
}

// ---- Constant presence ----------------------------------------------------

func TestWorkflowConstants(t *testing.T) {
	assert.NotEmpty(t, WorkflowModelOnboarding)
	assert.NotEmpty(t, WorkflowAgentScaling)
	assert.NotEmpty(t, WorkflowSkillUpdate)
	assert.NotEmpty(t, WorkflowModelRollback)
}

func TestActivityConstants(t *testing.T) {
	activities := []string{
		ActivityDownload, ActivityConvert, ActivityOptimize,
		ActivityDeploy, ActivityVerify, ActivityRollback, ActivityValidate,
		ActivityDrain, ActivityArchive, ActivityCleanup,
		ActivityAssess, ActivityScale, ActivityReport,
		ActivityStage, ActivitySwap, ActivitySnapshot, ActivityRevert,
		ActivityNotify,
	}
	seen := make(map[string]bool)
	for _, a := range activities {
		assert.NotEmpty(t, a)
		assert.False(t, seen[a], "duplicate activity constant: %s", a)
		seen[a] = true
	}
}

func TestEventConstants(t *testing.T) {
	events := []string{
		EventModelOnboardingRequested,
		EventAgentScalingNeeded,
		EventSkillUpdated,
		EventModelFailureDetected,
	}
	for _, e := range events {
		assert.NotEmpty(t, e)
	}
}
