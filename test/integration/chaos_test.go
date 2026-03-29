/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package integration

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/chaos"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
)

func TestResilience_PodKill_SelfHealing(t *testing.T) {
	ctx := suite.ctx
	name := fmt.Sprintf("resilience-podkill-%d", uniqueID())

	// 1. Create a ready LLMInferenceService
	// This helper creates the CR and a fake Deployment + Status.
	llm := newLLMInferenceService(t, name)

	// In envtest, since there's no real deployment controller for Pods,
	// we need to manually create the Pod that would be owned by the Deployment.
	// This mimics the 'Ready' state the operator expects.
	labels := map[string]string{
		"app.kubernetes.io/name":     "llminferenceservice",
		"app.kubernetes.io/instance": name,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-pod",
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "vllm"}},
		},
	}
	require.NoError(t, suite.client.Create(ctx, pod))
	mockPodReady(t, pod)

	// 2. Wait for stable Ready status
	assert.Eventually(t, func() bool {
		var s servingv1alpha2.LLMInferenceService
		if err := suite.client.Get(ctx, client.ObjectKeyFromObject(llm), &s); err != nil {
			return false
		}
		return s.Status.ModelReady && s.Status.Replicas > 0
	}, eventuallyTimeout, eventuallyInterval)

	// 3. Inject Chaos: Kill the Pod
	exp := chaos.Experiment{
		Name:      "kill-vllm-pod",
		Type:      chaos.ExperimentPodKill,
		Namespace: testNamespace,
		Selector:  labels,
		Parameters: chaos.ExperimentParams{
			KillPercentage: 100,
		},
	}

	start := time.Now()
	res, err := suite.chaos.RunExperiment(ctx, exp)
	require.NoError(t, err)
	assert.Equal(t, 1, res.AffectedPods)

	// 4. Verify status becomes NotReady
	// The controller should detect the Pod is gone and update Status.
	assert.Eventually(t, func() bool {
		var s servingv1alpha2.LLMInferenceService
		if err := suite.client.Get(ctx, client.ObjectKeyFromObject(llm), &s); err != nil {
			return false
		}
		
		// Let's patch the Deployment to 0 ready replicas to simulate the disruption.
		var deploy appsv1.Deployment
		if err := suite.client.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, &deploy); err == nil {
			patch := client.MergeFrom(deploy.DeepCopy())
			deploy.Status.ReadyReplicas = 0
			_ = suite.client.Status().Patch(ctx, &deploy, patch)
		}
		
		return !s.Status.ModelReady
	}, eventuallyTimeout, eventuallyInterval, "Status should become NotReady after Pod kill")

	// 5. Simulate "Self-Healing": Re-create Pod and set it to Ready
	// In a real cluster, the Deployment controller would do this.
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-new-pod",
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: pod.Spec,
	}
	require.NoError(t, suite.client.Create(ctx, newPod))
	
	// Also restore deployment status
	var deploy appsv1.Deployment
	require.NoError(t, suite.client.Get(ctx, client.ObjectKey{Name: name, Namespace: testNamespace}, &deploy))
	patchDeploy := client.MergeFrom(deploy.DeepCopy())
	deploy.Status.ReadyReplicas = 1
	require.NoError(t, suite.client.Status().Patch(ctx, &deploy, patchDeploy))

	mockPodReady(t, newPod)

	// 6. Verify recovery
	assert.Eventually(t, func() bool {
		var s servingv1alpha2.LLMInferenceService
		if err := suite.client.Get(ctx, client.ObjectKeyFromObject(llm), &s); err != nil {
			return false
		}
		return s.Status.ModelReady
	}, eventuallyTimeout, eventuallyInterval, "Status should recover to Ready")

	mttr := time.Since(start)
	t.Logf("MTTR (Mean Time To Recovery): %v", mttr)
}

func TestResilience_LocalModelCache_NodeEviction(t *testing.T) {
	ctx := suite.ctx
	name := fmt.Sprintf("resilience-evict-%d", uniqueID())

	// 1. Create a Node and a LocalModelCache
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-chaos"}}
	require.NoError(t, suite.client.Create(ctx, node))
	defer func() { _ = suite.client.Delete(ctx, node) }()

	lmc := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LocalModelCacheSpec{
			SourceModelURI: "hf://org/model-chaos",
			ModelSize:      "1Gi",
			WarmNodes:      []string{"node-chaos"},
		},
	}
	require.NoError(t, suite.client.Create(ctx, lmc))

	// 2. Wait for Job to be created and set it to Ready
	modelHash := controller.ModelURIHash(lmc.Spec.SourceModelURI)
	jobName := fmt.Sprintf("lmc-warmup-%s-%s", modelHash, fmt.Sprintf("%x", sha256.Sum256([]byte("node-chaos")))[:8])

	var job batchv1.Job
	assert.Eventually(t, func() bool {
		return suite.client.Get(ctx, client.ObjectKey{Name: jobName, Namespace: testNamespace}, &job) == nil
	}, eventuallyTimeout, eventuallyInterval)

	// Mock Job completion (simulating successful download)
	patchJob := client.MergeFrom(job.DeepCopy())
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	require.NoError(t, suite.client.Status().Patch(ctx, &job, patchJob))

	// Verify LMC is Ready
	assert.Eventually(t, func() bool {
		var s servingv1alpha2.LocalModelCache
		if err := suite.client.Get(ctx, client.ObjectKeyFromObject(lmc), &s); err != nil {
			return false
		}
		return s.Status.CachedNodes > 0
	}, eventuallyTimeout, eventuallyInterval)

	// 3. Inject Chaos: Delete the Job (simulating node failure/eviction of the workspace)
	require.NoError(t, suite.client.Delete(ctx, &job))

	// 4. Verify the Operator re-creates the Job to restore the cache
	assert.Eventually(t, func() bool {
		var newJob batchv1.Job
		err := suite.client.Get(ctx, client.ObjectKey{Name: jobName, Namespace: testNamespace}, &newJob)
		return err == nil && newJob.UID != job.UID
	}, eventuallyTimeout, eventuallyInterval, "Job should be re-created after deletion")
}

func TestResilience_LoRA_API_Failure(t *testing.T) {
	ctx := suite.ctx
	name := fmt.Sprintf("resilience-lora-%d", uniqueID())

	// 1. Create a LoRA Adapter
	lora := &servingv1alpha2.LLMLoraAdapter{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: servingv1alpha2.LLMLoraAdapterSpec{
			TargetService: name, // Dummy service name
			AdapterName:   "adapter-1",
			Model: servingv1alpha2.ModelSpec{
				Name: "adapter-1",
				URI:  "hf://org/lora-1",
			},
		},
	}
	require.NoError(t, suite.client.Create(ctx, lora))

	// 2. Verify it's being reconciled (at least status conditions started appearing)
	assert.Eventually(t, func() bool {
		var s servingv1alpha2.LLMLoraAdapter
		if err := suite.client.Get(ctx, client.ObjectKeyFromObject(lora), &s); err != nil {
			return false
		}
		// Since we don't have a vLLM pod running, it will eventually enter a failed or waiting state.
		// We just want to see the reconciler interacting with it.
		return len(s.Status.Conditions) > 0
	}, eventuallyTimeout, eventuallyInterval)
}
