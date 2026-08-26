/* Copyright 2026 CKodex Authors. Licensed under the Apache License, Version 2.0. */
package controller

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

func newEvaluationReconciler(t *testing.T, objects ...client.Object) (*LLMEvaluationReconciler, client.Client) {
	t.Helper()
	scheme := buildLoraScheme(t)
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(&servingv1alpha2.LLMLoraAdapter{}).Build()
	return &LLMEvaluationReconciler{
		Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(20),
		AuditLogger: observability.NewAuditLoggerWithOptions(cl, scheme, false),
	}, cl
}

func evaluationRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: name, Namespace: "default"}}
}

func TestLLMEvaluationReconciler_ReconcileNotFound(t *testing.T) {
	r, _ := newEvaluationReconciler(t)
	result, err := r.Reconcile(context.Background(), evaluationRequest("missing"))
	if err != nil || result != (ctrl.Result{}) {
		t.Fatalf("missing adapter: result=%+v err=%v", result, err)
	}
}

func TestLLMEvaluationReconciler_ReconcileIgnoresNonPending(t *testing.T) {
	adapter := testLora("active", "default", "target")
	adapter.Status.StatePlanes.Lifecycle = "active"
	r, _ := newEvaluationReconciler(t, adapter)
	if _, err := r.Reconcile(context.Background(), evaluationRequest(adapter.Name)); err != nil {
		t.Fatalf("non-pending adapter: %v", err)
	}
}

func TestLLMEvaluationReconciler_ReconcileCreatesEvaluationJob(t *testing.T) {
	adapter := testLora("pending", "default", "target")
	adapter.Status.StatePlanes.Lifecycle = "pending-evaluation"
	r, cl := newEvaluationReconciler(t, adapter)
	if _, err := r.Reconcile(context.Background(), evaluationRequest(adapter.Name)); err != nil {
		t.Fatalf("create evaluation job: %v", err)
	}
	var job batchv1.Job
	if err := cl.Get(context.Background(), k8stypes.NamespacedName{Name: "eval-pending", Namespace: "default"}, &job); err != nil {
		t.Fatalf("created job: %v", err)
	}
	if job.Spec.Template.Spec.Containers[0].Args[1] != "--adapter" || job.Labels["serving.ckodex.com/adapter-eval"] != adapter.Name {
		t.Fatalf("unexpected evaluation job: %+v", job.Spec.Template.Spec.Containers[0])
	}
}

func TestLLMEvaluationReconciler_ReconcileTracksRunningJob(t *testing.T) {
	adapter := testLora("running", "default", "target")
	adapter.Status.StatePlanes.Lifecycle = "pending-evaluation"
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "eval-running", Namespace: "default", Labels: map[string]string{"serving.ckodex.com/adapter-eval": adapter.Name}}}
	r, _ := newEvaluationReconciler(t, adapter, job)
	if _, err := r.Reconcile(context.Background(), evaluationRequest(adapter.Name)); err != nil {
		t.Fatalf("running job: %v", err)
	}
}

func TestLLMEvaluationReconciler_ReconcilePromotesAndQuarantines(t *testing.T) {
	for _, tc := range []struct {
		name              string
		succeeded, failed int32
		lifecycle, trust  string
	}{
		{"succeeded", 1, 0, "active", "asserted"},
		{"failed", 0, 1, "quarantined", "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := testLora(tc.name, "default", "target")
			adapter.Status.StatePlanes.Lifecycle = "pending-evaluation"
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "eval-" + tc.name, Namespace: "default", Labels: map[string]string{"serving.ckodex.com/adapter-eval": tc.name}}, Status: batchv1.JobStatus{Succeeded: tc.succeeded, Failed: tc.failed}}
			r, cl := newEvaluationReconciler(t, adapter, job)
			if _, err := r.Reconcile(context.Background(), evaluationRequest(tc.name)); err != nil {
				t.Fatalf("terminal job: %v", err)
			}
			var updated servingv1alpha2.LLMLoraAdapter
			if err := cl.Get(context.Background(), client.ObjectKeyFromObject(adapter), &updated); err != nil {
				t.Fatal(err)
			}
			if updated.Status.StatePlanes.Lifecycle != tc.lifecycle || updated.Status.StatePlanes.Trust != tc.trust {
				t.Fatalf("status=%+v", updated.Status.StatePlanes)
			}
		})
	}
}

func TestLLMEvaluationReconciler_LaunchJobRequiresScheme(t *testing.T) {
	adapter := testLora("invalid", "default", "target")
	adapter.Status.StatePlanes.Lifecycle = "pending-evaluation"
	r, _ := newEvaluationReconciler(t, adapter)
	r.Client = &evaluationCreateErrorClient{Client: r.Client}
	if _, err := r.Reconcile(context.Background(), evaluationRequest(adapter.Name)); err == nil {
		t.Fatal("expected job creation error")
	}
}

type evaluationCreateErrorClient struct {
	client.Client
}

func (c *evaluationCreateErrorClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return context.Canceled
}
