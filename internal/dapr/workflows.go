/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package dapr implements Dapr workflow definitions for agent auto-management.
package dapr

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WorkflowName constants for Dapr workflow definitions.
const (
	WorkflowModelOnboarding = "model-onboarding"
	WorkflowAgentScaling    = "agent-scaling"
	WorkflowSkillUpdate     = "skill-update"
	WorkflowModelRollback   = "model-rollback"
)

// ActivityName constants for workflow activities.
const (
	ActivityDownload = "download-model"
	ActivityConvert  = "convert-model"
	ActivityOptimize = "optimize-model"
	ActivityDeploy   = "deploy-model"
	ActivityVerify   = "verify-model"
	ActivityRollback = "rollback-model"
	ActivityValidate = "validate-model"
	ActivityDrain    = "drain-model"
	ActivityArchive  = "archive-model"
	ActivityCleanup  = "cleanup-model"
	ActivityAssess   = "assess-scaling"
	ActivityScale    = "scale-replicas"
	ActivityReport   = "report-scaling"
	ActivityStage    = "stage-skill"
	ActivitySwap     = "swap-skill"
	ActivitySnapshot = "snapshot-state"
	ActivityRevert   = "revert-state"
	ActivityNotify   = "notify-status"
)

// EventType constants for CloudEvents → Dapr pub/sub triggers.
const (
	EventModelOnboardingRequested = "model.onboarding.requested"
	EventAgentScalingNeeded       = "agent.scaling.needed"
	EventSkillUpdated             = "skill.updated"
	EventModelFailureDetected     = "model.failure.detected"
)

// WorkflowInput is the common input for all workflows.
type WorkflowInput struct {
	ModelRef    string            `json:"modelRef"`
	Namespace   string            `json:"namespace"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	RequestedBy string            `json:"requestedBy"`
	Timestamp   time.Time         `json:"timestamp"`
}

// WorkflowResult is the common result from all workflows.
type WorkflowResult struct {
	WorkflowID string        `json:"workflowId"`
	Status     string        `json:"status"` // completed, failed, compensated, compensation-failed
	Activities []ActivityLog `json:"activities"`
	Duration   time.Duration `json:"duration"`
}

// ActivityLog records the execution of a single activity.
type ActivityLog struct {
	Name     string        `json:"name"`
	Status   string        `json:"status"` // completed, failed, compensated, compensation-failed
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// ----- Workflow Definitions -----

// ModelOnboardingWorkflow orchestrates the model lifecycle:
// download → convert → optimize → deploy → verify
// With compensation (saga pattern): rollback on any failure.
type ModelOnboardingWorkflow struct {
	Input WorkflowInput
}

// Steps returns the ordered activity definitions for this workflow.
func (w *ModelOnboardingWorkflow) Steps() []WorkflowStep {
	return []WorkflowStep{
		{Activity: ActivityValidate, Compensate: ""},
		{Activity: ActivityDownload, Compensate: ActivityCleanup},
		{Activity: ActivityConvert, Compensate: ActivityCleanup},
		{Activity: ActivityOptimize, Compensate: ActivityCleanup},
		{Activity: ActivityDeploy, Compensate: ActivityRollback},
		{Activity: ActivityVerify, Compensate: ActivityRollback},
	}
}

// AgentScalingWorkflow handles dynamic agent scaling:
// assess → scale → verify → report
type AgentScalingWorkflow struct {
	Input WorkflowInput
}

func (w *AgentScalingWorkflow) Steps() []WorkflowStep {
	return []WorkflowStep{
		{Activity: ActivityAssess, Compensate: ""},
		{Activity: ActivityScale, Compensate: ActivityRevert},
		{Activity: ActivityVerify, Compensate: ActivityRevert},
		{Activity: ActivityReport, Compensate: ""},
	}
}

// SkillUpdateWorkflow validates and swaps a skill version:
// validate → stage → swap → verify
type SkillUpdateWorkflow struct {
	Input WorkflowInput
}

func (w *SkillUpdateWorkflow) Steps() []WorkflowStep {
	return []WorkflowStep{
		{Activity: ActivityValidate, Compensate: ""},
		{Activity: ActivityStage, Compensate: ActivityCleanup},
		{Activity: ActivitySwap, Compensate: ActivityRevert},
		{Activity: ActivityVerify, Compensate: ActivityRevert},
	}
}

// ModelRollbackWorkflow handles rollback on failure:
// snapshot → revert → verify → notify
type ModelRollbackWorkflow struct {
	Input WorkflowInput
}

func (w *ModelRollbackWorkflow) Steps() []WorkflowStep {
	return []WorkflowStep{
		{Activity: ActivitySnapshot, Compensate: ""},
		{Activity: ActivityRevert, Compensate: ""},
		{Activity: ActivityVerify, Compensate: ""},
		{Activity: ActivityNotify, Compensate: ""},
	}
}

// WorkflowStep defines a single step with optional compensation.
type WorkflowStep struct {
	Activity   string // Activity to execute
	Compensate string // Activity to run on rollback (empty = no compensation)
}

// ----- Saga Executor -----

// ActivityFunc is the function signature for workflow activities.
type ActivityFunc func(ctx context.Context, input WorkflowInput) error

// SagaExecutor runs workflow steps with compensation on failure.
type SagaExecutor struct {
	Activities map[string]ActivityFunc
}

// Execute runs a workflow's steps, compensating on failure.
func (s *SagaExecutor) Execute(ctx context.Context, name string, steps []WorkflowStep, input WorkflowInput) (*WorkflowResult, error) {
	result := &WorkflowResult{WorkflowID: name, Status: "running"}
	start := time.Now()
	var completedSteps []WorkflowStep

	for _, step := range steps {
		actStart := time.Now()
		fn, ok := s.Activities[step.Activity]
		if !ok {
			result.Status = "failed"
			result.Duration = time.Since(start)
			return result, fmt.Errorf("unknown activity: %s", step.Activity)
		}

		if err := fn(ctx, input); err != nil {
			// Record failure
			result.Activities = append(result.Activities, ActivityLog{
				Name: step.Activity, Status: "failed",
				Duration: time.Since(actStart), Error: err.Error(),
			})

			compensationErrors := s.compensate(ctx, input, completedSteps, result)
			if len(compensationErrors) > 0 {
				result.Status = "compensation-failed"
				result.Duration = time.Since(start)
				return result, fmt.Errorf("workflow %s failed at %s: %w (compensation failures: %w)", name, step.Activity, err, errors.Join(compensationErrors...))
			}
			result.Status = "compensated"
			result.Duration = time.Since(start)
			return result, fmt.Errorf("workflow %s failed at %s: %w", name, step.Activity, err)
		}

		result.Activities = append(result.Activities, ActivityLog{
			Name: step.Activity, Status: "completed", Duration: time.Since(actStart),
		})
		completedSteps = append(completedSteps, step)
	}

	result.Status = "completed"
	result.Duration = time.Since(start)
	return result, nil
}

func (s *SagaExecutor) compensate(ctx context.Context, input WorkflowInput, steps []WorkflowStep, result *WorkflowResult) []error {
	var compensationErrors []error
	for i := len(steps) - 1; i >= 0; i-- {
		name := steps[i].Compensate
		if name == "" {
			continue
		}
		compFn, ok := s.Activities[name]
		if !ok {
			err := fmt.Errorf("compensation activity %s is not registered", name)
			compensationErrors = append(compensationErrors, err)
			result.Activities = append(result.Activities, ActivityLog{Name: name, Status: "compensation-failed", Error: err.Error()})
			continue
		}
		if err := compFn(ctx, input); err != nil {
			compensationErrors = append(compensationErrors, fmt.Errorf("%s: %w", name, err))
			result.Activities = append(result.Activities, ActivityLog{Name: name, Status: "compensation-failed", Error: err.Error()})
			continue
		}
		result.Activities = append(result.Activities, ActivityLog{Name: name, Status: "compensated"})
	}
	return compensationErrors
}
