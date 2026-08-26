package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestLocalModelCacheCoverageJobPhases(t *testing.T) {
	complete := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	failed := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}}}
	active := &batchv1.Job{Status: batchv1.JobStatus{Active: 1}}
	for _, test := range []struct {
		name string
		job  *batchv1.Job
		want string
	}{
		{name: "complete", job: complete, want: "Ready"},
		{name: "failed", job: failed, want: "Failed"},
		{name: "active", job: active, want: "Downloading"},
		{name: "pending", job: &batchv1.Job{}, want: "Pending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := jobPhase(test.job); got != test.want {
				t.Fatalf("jobPhase() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLocalModelCacheCoverageReadyJobPreservesLastUse(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	oldUse := v1.NewTime(time.Now().Add(-time.Hour))
	lmc := makeLocalModelCache("cache")
	lmc.Status.NodeStatuses = []servingv1alpha2.NodeCacheStatus{{NodeName: "node-a", Phase: "Ready", LastUsed: &oldUse}}
	start := v1.NewTime(time.Now().Add(-2 * time.Minute))
	finish := v1.NewTime(time.Now().Add(-time.Minute))
	job := &batchv1.Job{Status: batchv1.JobStatus{StartTime: &start, CompletionTime: &finish, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	status := pendingNodeCacheStatus("node-a", "pvc", "hash")
	r := &LocalModelCacheReconciler{Scheme: scheme, Recorder: record.NewFakeRecorder(5)}
	now := v1.Now()
	r.updateNodeCacheFromJob(context.Background(), lmc, &status, job, "node-a", "job", now)
	modelSize := lmc.Spec.ModelSizeQuantity()
	if status.Phase != "Ready" || status.SizeBytes != modelSize.Value() {
		t.Fatalf("ready status = %#v", status)
	}
	if !status.LastUsed.Time.Equal(oldUse.Time) {
		t.Fatalf("last use changed: got %v, want %v", status.LastUsed, oldUse)
	}

	lmc.Status.NodeStatuses = nil
	status = pendingNodeCacheStatus("node-b", "pvc", "hash")
	r.updateNodeCacheFromJob(context.Background(), lmc, &status, job, "node-b", "job", now)
	if !status.LastUsed.Time.Equal(now.Time) {
		t.Fatalf("new cache last use = %v, want %v", status.LastUsed, now)
	}
}

func TestLocalModelCacheCoverageFailedJobSelfHealing(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("cache")
	recorder := record.NewFakeRecorder(5)
	job := &batchv1.Job{ObjectMeta: v1.ObjectMeta{Name: "job", Namespace: "default"}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: v1.NewTime(time.Now().Add(-10 * time.Minute))}}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &LocalModelCacheReconciler{Client: cl, Scheme: scheme, Recorder: recorder}
	status := pendingNodeCacheStatus("node", "pvc", "hash")
	r.updateNodeCacheFromJob(context.Background(), lmc, &status, job, "node", "job", v1.Now())
	if status.Phase != "Pending" {
		t.Fatalf("self-healed phase = %q", status.Phase)
	}
	if err := cl.Get(context.Background(), cacheClientKey("default", "job"), &batchv1.Job{}); err == nil {
		t.Fatal("failed job still exists")
	}

	recent := job.DeepCopy()
	recent.Status.Conditions[0].LastTransitionTime = v1.NewTime(time.Now().Add(-time.Minute))
	status = pendingNodeCacheStatus("node", "pvc", "hash")
	r.selfHealFailedJob(context.Background(), lmc, &status, recent, "node", "job")
	if status.Phase != "Pending" {
		t.Fatalf("recent failure phase = %q", status.Phase)
	}
}

func cacheClientKey(namespace, name string) client.ObjectKey {
	return client.ObjectKey{Namespace: namespace, Name: name}
}

func TestLocalModelCacheCoverageFailedJobTime(t *testing.T) {
	job := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	if failedJobTime(job) != nil {
		t.Fatal("successful job has failed time")
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionFalse}}
	if failedJobTime(job) != nil {
		t.Fatal("false failure has failed time")
	}
}

func TestLocalModelCacheCoverageWarmupJobExistingAndDeleting(t *testing.T) {
	scheme := buildLocalModelCacheScheme(t)
	lmc := makeLocalModelCache("cache")
	activeJob := &batchv1.Job{ObjectMeta: v1.ObjectMeta{Name: "active", Namespace: "default"}, Status: batchv1.JobStatus{Active: 1}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(activeJob).Build()
	r := &LocalModelCacheReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(5)}
	status := pendingNodeCacheStatus("node", "pvc", "hash")
	job, err := r.ensureWarmupJob(context.Background(), lmc, &status, "active", "pvc", "default", "node", v1.Now())
	if err != nil || job == nil {
		t.Fatalf("existing warmup job = %#v, err=%v", job, err)
	}

	deleting := activeJob.DeepCopy()
	deleting.Name = "deleting"
	deleting.DeletionTimestamp = &v1.Time{Time: time.Now()}
	r.APIReader = localModelCacheJobReader{job: deleting}
	job, err = r.ensureWarmupJob(context.Background(), lmc, &status, "deleting", "pvc", "default", "node", v1.Now())
	if err != nil || job != nil {
		t.Fatalf("deleting warmup job = %#v, err=%v", job, err)
	}
}

func TestLocalModelCacheCoverageCacheLookupErrors(t *testing.T) {
	lmc := makeLocalModelCache("cache")
	r := &LocalModelCacheReconciler{APIReader: localModelCacheErrorReader{err: fmt.Errorf("lookup failed")}}
	status := pendingNodeCacheStatus("node", "pvc", "hash")
	if _, err := r.ensureCachePVC(context.Background(), lmc, &status, "pvc", "default", "node", "hash", "job", v1.Now()); err == nil {
		t.Fatal("expected PVC lookup error")
	}
	if _, err := r.ensureWarmupJob(context.Background(), lmc, &status, "job", "pvc", "default", "node", v1.Now()); err == nil {
		t.Fatal("expected Job lookup error")
	}
}

type localModelCacheJobReader struct{ job *batchv1.Job }

func (r localModelCacheJobReader) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	obj.(*batchv1.Job).ObjectMeta = *r.job.ObjectMeta.DeepCopy()
	obj.(*batchv1.Job).Status = *r.job.Status.DeepCopy()
	return nil
}

func (localModelCacheJobReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return nil
}
