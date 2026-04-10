/*
Copyright 2026 CKodex Authors.
*/

package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// LLMEvaluationReconciler handles asynchronous model benchmarking and safety verification via Jobs.
type LLMEvaluationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	AuditLogger *observability.AuditLogger
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *LLMEvaluationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the adapter
	var adapter servingv1alpha2.LLMLoraAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Only process adapters in 'pending-evaluation' state
	if adapter.Status.StatePlanes.Lifecycle != "pending-evaluation" {
		return ctrl.Result{}, nil
	}

	// 3. Check if an evaluation job is already running
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(adapter.Namespace), client.MatchingLabels{"serving.ckodex.com/adapter-eval": adapter.Name}); err == nil {
		if len(jobList.Items) > 0 {
			// Job already exists, track its progress (Wait for completion)
			job := jobList.Items[0]
			if job.Status.Succeeded > 0 {
				logger.Info("Evaluation job succeeded, promoting adapter", "adapter", adapter.Name)
				return r.finalizeEvaluation(ctx, &adapter, true)
			}
			if job.Status.Failed > 0 {
				logger.Error(nil, "Evaluation job failed", "adapter", adapter.Name)
				return r.finalizeEvaluation(ctx, &adapter, false)
			}
			return ctrl.Result{}, nil
		}
	}

	// 4. Trigger new Evaluation Job
	logger.Info("Starting asynchronous evaluation benchmark via Job", "adapter", adapter.Name)
	if err := r.launchEvalJob(ctx, &adapter); err != nil {
		r.Recorder.Event(&adapter, corev1.EventTypeWarning, "EvaluationTriggerFailed", fmt.Sprintf("Failed to launch eval job: %v", err))
		return ctrl.Result{}, fmt.Errorf("launch eval job: %w", err)
	}

	r.AuditLogger.LogUpdate(ctx, "LLMLoraAdapter", adapter.Name, map[string]string{
		"action": "EvaluationStarted",
		"reason": "Asynchronous Job-based safety benchmark initiated",
	})
	r.Recorder.Event(&adapter, corev1.EventTypeNormal, "EvaluationStarted", "Asynchronous Job-based safety benchmark initiated")

	return ctrl.Result{}, nil
}

func (r *LLMEvaluationReconciler) launchEvalJob(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) error {
	jobName := fmt.Sprintf("eval-%s", adapter.Name)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: adapter.Namespace,
			Labels: map[string]string{
				"serving.ckodex.com/adapter-eval": adapter.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptrToInt32(2),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "eval-runner",
							Image: api.CKodexStorageInitializerImage, // Placeholder
							Args: []string{
								"run-eval",
								"--adapter", adapter.Name,
								"--output-path", "/tmp/eval-report.json",
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(adapter, job, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, job)
}

func (r *LLMEvaluationReconciler) finalizeEvaluation(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter, success bool) (ctrl.Result, error) {
	patch := client.MergeFrom(adapter.DeepCopy())

	if success {
		adapter.Status.StatePlanes.Trust = "verified"
		adapter.Status.StatePlanes.Lifecycle = "active" // Automatic promotion if verified
		now := metav1.Now()
		adapter.Status.EvidenceBundle.LastVerifiedAt = &now
	} else {
		adapter.Status.StatePlanes.Lifecycle = "quarantined"
		adapter.Status.StatePlanes.Trust = "denied"
	}

	if err := r.Status().Patch(ctx, adapter, patch); err != nil {
		return ctrl.Result{}, err
	}

	r.AuditLogger.LogUpdate(ctx, "LLMLoraAdapter", adapter.Name, map[string]string{
		"action":  "EvaluationCompleted",
		"success": fmt.Sprintf("%t", success),
	})

	return ctrl.Result{}, nil
}

func ptrToInt32(i int32) *int32 {
	return &i
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMEvaluationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("ckodex-evaluator")
	return ctrl.NewControllerManagedBy(mgr).
		Named("llmevaluation").
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
