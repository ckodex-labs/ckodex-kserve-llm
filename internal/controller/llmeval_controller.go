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
)

// LLMEvalReconciler reconciles the evaluation of LLMLoraAdapters.
type LLMEvalReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=serving.ckodex.com,resources=llmloraadapters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=serving.ckodex.com,resources=evalprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *LLMEvalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the adapter
	var adapter servingv1alpha2.LLMLoraAdapter
	if err := r.Get(ctx, req.NamespacedName, &adapter); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Only trigger eval for adapters in 'proposed' state
	if adapter.Status.StatePlanes.Lifecycle != "proposed" {
		return ctrl.Result{}, nil
	}

	// 3. Check if an eval job is already running
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(adapter.Namespace), client.MatchingLabels{"serving.ckodex.com/adapter-eval": adapter.Name}); err == nil {
		if len(jobList.Items) > 0 {
			// Job already exists, let it complete
			return ctrl.Result{}, nil
		}
	}

	// 4. Trigger new Eval Job
	logger.Info("triggering automated evaluation for adapter", "name", adapter.Name)
	if err := r.launchEvalJob(ctx, &adapter); err != nil {
		return ctrl.Result{}, fmt.Errorf("launch eval job: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *LLMEvalReconciler) launchEvalJob(ctx context.Context, adapter *servingv1alpha2.LLMLoraAdapter) error {
	jobName := fmt.Sprintf("eval-%s", adapter.Name)
	
	// Create Job spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: adapter.Namespace,
			Labels: map[string]string{
				"serving.ckodex.com/adapter-eval": adapter.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "eval-runner",
							Image: api.CKodexStorageInitializerImage, // Placeholder for actual runner image
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

func (r *LLMEvalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha2.LLMLoraAdapter{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
