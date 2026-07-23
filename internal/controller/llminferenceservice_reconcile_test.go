/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/cleanup"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/deployment"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/evidence"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/reconciler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/status"
	kserveintegration "github.com/ckodex-labs/kserve-llm-operator/internal/kserve"
)

func buildLLMScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, autoscalingv2.AddToScheme(s))
	require.NoError(t, policyv1.AddToScheme(s))
	require.NoError(t, networkingv1.AddToScheme(s))
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, gwapiv1.Install(s))
	return s
}

func TestLLMInferenceService_ReconcileMultiNodeDelegatesToKServe(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("distributed", "default")
	llmSvc.Spec.Model.URI = "pvc://model-weights"
	llmSvc.Spec.Worker = &servingv1alpha2.WorkerSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "worker"}},
			},
		},
	}
	llmSvc.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor:   ptr32(2),
		Pipeline: ptr32(2),
	}
	modelPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "model-weights", Namespace: llmSvc.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc, modelPVC).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)
	r.KServeMultiNode = &kserveintegration.Reconciler{
		Client: cl,
		Scheme: s,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: llmSvc.Name, Namespace: llmSvc.Namespace},
	})
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, result.RequeueAfter)

	isvc := kserveintegration.NewInferenceService()
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: llmSvc.Name, Namespace: llmSvc.Namespace,
	}, isvc))

	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: llmSvc.Name, Namespace: llmSvc.Namespace,
	}, &appsv1.Deployment{}))
	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: llmSvc.Name, Namespace: llmSvc.Namespace,
	}, &corev1.Service{}))
	require.Error(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: llmSvc.Name, Namespace: llmSvc.Namespace,
	}, &policyv1.PodDisruptionBudget{}))
}

func makeLLMInferenceService(name, namespace string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       k8stypes.UID("test-uid-" + name),
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				URI:  "hf://test/model",
				Name: "test-model",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "vllm"},
					},
				},
			},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{
						GatewayClassName: "envoy",
					},
				},
			},
		},
	}
}

func setupReconciler(cl client.Client, s *runtime.Scheme) *LLMInferenceServiceReconciler {
	rec := record.NewFakeRecorder(10)
	return &LLMInferenceServiceReconciler{
		Client:   cl,
		Scheme:   s,
		Recorder: rec,
		DeploymentBuilder: &deployment.Builder{
			Client:   cl,
			Recorder: rec,
		},
		StatusReconciler: &status.Reconciler{
			Client:          cl,
			EnableHardening: true,
		},
		CleanupReconciler: &cleanup.Reconciler{
			Client: cl,
		},
		ServiceReconciler: &reconciler.ServiceReconciler{
			Client:     cl,
			Scheme:     s,
			EnableGRPC: false,
		},
		PDBReconciler: &reconciler.PDBReconciler{
			Client: cl,
			Scheme: s,
		},
		GovernanceReconciler: &evidence.GovernanceReconciler{
			Client: cl,
		},
	}
}

// TestLLMInferenceService_ReconcileNotFound returns no error when CR is missing.
func TestLLMInferenceService_ReconcileNotFound(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestLLMInferenceService_ReconcileCreatesDeploymentServicePDB verifies the main
// reconcile loop creates Deployment, Service, and PDB for a new CR.
func TestLLMInferenceService_ReconcileCreatesDeploymentServicePDB(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-llm", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Deployment should exist.
	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &deploy))

	// Service should exist.
	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))

	// PDB should exist.
	var pdb policyv1.PodDisruptionBudget
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &pdb))

	// Finalizer should be present on the CR.
	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &updated))
	assert.Contains(t, updated.Finalizers, api.FinalizerName)
}

// TestLLMInferenceService_ReconcileDeletion exercises the finalizer cleanup path.
func TestLLMInferenceService_ReconcileDeletion(t *testing.T) {
	s := buildLLMScheme(t)
	// Create the object first without a deletion timestamp, then patch it.
	llmSvc := makeLLMInferenceService("my-llm", "default")
	llmSvc.Finalizers = []string{api.FinalizerName}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()

	// Mark for deletion by calling Delete on the fake client (sets DeletionTimestamp).
	require.NoError(t, cl.Delete(context.Background(), llmSvc))

	r := setupReconciler(cl, s)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "my-llm", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestReconcileDeployment_CreatesNew verifies deployment creation.
func TestReconcileDeployment_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &deploy))
	assert.Equal(t, "my-llm", deploy.Name)
}

// TestReconcileDeployment_Gemma4WellKnown verifies that Gemma-4 gets optimized defaults.
func TestReconcileDeployment_Gemma4WellKnown(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("gemma-svc", "default")
	llmSvc.Spec.Model.URI = "hf://google/gemma-4-E2B-it"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "gemma-svc", Namespace: "default",
	}, &deploy))

	// Verify vLLM args from WellKnown config
	vllmContainer := deploy.Spec.Template.Spec.Containers[0]
	args := strings.Join(vllmContainer.Args, " ")
	assert.Contains(t, args, "--max-model-len 131072")
	assert.Contains(t, args, "--trust-remote-code")
	assert.Contains(t, args, "--model /mnt/models")
	assert.Equal(t, api.VLLMImage, vllmContainer.Image)

	// Verify resources from WellKnown config (requests match our defined defaults)
	assert.Equal(t, resource.MustParse("8"), vllmContainer.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("32Gi"), vllmContainer.Resources.Requests[corev1.ResourceMemory])
}

// TestReconcileDeployment_OCIGemma4Optimization verifies URI-agnostic matching for OCI.
func TestReconcileDeployment_OCIGemma4Optimization(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("oci-gemma", "default")
	llmSvc.Spec.Model.URI = "oci://ghcr.io/ckodex-labs/models/gemma-4-e2b-it:latest"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "oci-gemma", Namespace: "default",
	}, &deploy))

	// Verify vLLM args are optimized even for OCI URI
	vllmContainer := deploy.Spec.Template.Spec.Containers[0]
	args := strings.Join(vllmContainer.Args, " ")
	assert.Contains(t, args, "--model /mnt/models")
}

// TestReconcileDeployment_PVCNativeMount verifies that pvc:// URI results in direct volume mount.
func TestReconcileDeployment_PVCNativeMount(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("pvc-svc", "default")
	llmSvc.Spec.Model.URI = "pvc://existing-model-pvc"

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)

	var deploy appsv1.Deployment
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "pvc-svc", Namespace: "default",
	}, &deploy))

	// Verify no init container
	assert.Empty(t, deploy.Spec.Template.Spec.InitContainers)

	// Verify PVC volume source
	found := false
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == api.ModelVolumeName {
			require.NotNil(t, v.PersistentVolumeClaim)
			assert.Equal(t, "existing-model-pvc", v.PersistentVolumeClaim.ClaimName)
			found = true
			break
		}
	}
	assert.True(t, found)
}

// TestReconcileDeployment_UpdatesExisting exercises the update path.
func TestReconcileDeployment_UpdatesExisting(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	// Pre-create a deployment with different replica count.
	replicas := int32(3)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "vllm", Image: "old-image"}},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingDeploy).Build()
	r := setupReconciler(cl, s)

	err := r.reconcileDeployment(context.Background(), llmSvc, nil)
	require.NoError(t, err)
}

// TestReconcileService_CreatesNew verifies service creation.
func TestReconcileService_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
}

// TestReconcileService_GRPCPortAdded verifies gRPC port is added when enabled.
func TestReconcileService_GRPCPortAdded(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)
	r.ServiceReconciler.EnableGRPC = true

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	assert.Len(t, svc.Spec.Ports, 2)
	assert.Equal(t, "grpc-inference", svc.Spec.Ports[1].Name)
}

// TestReconcilePDB_CreatesNew verifies PodDisruptionBudget creation.
func TestReconcilePDB_CreatesNew(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc).Build()
	r := setupReconciler(cl, s)

	err := r.PDBReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var pdb policyv1.PodDisruptionBudget
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &pdb))
	assert.NotNil(t, pdb.Spec.MinAvailable)
}

// TestUpdateStatus_NoDeployment sets replicas=0 and ready=false when deployment missing.
func TestUpdateStatus_NoDeployment(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	err := r.StatusReconciler.Update(context.Background(), llmSvc, llmSvc.DeepCopy(), false, nil)
	require.NoError(t, err)
	assert.Equal(t, int32(0), llmSvc.Status.Replicas)
	assert.False(t, llmSvc.Status.ModelReady)
}

// TestUpdateStatus_WithReadyDeployment sets replicas and ready from deployment status.
func TestUpdateStatus_WithReadyDeployment(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	readyReplicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: readyReplicas},
	}

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc, deploy).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	err := r.StatusReconciler.Update(context.Background(), llmSvc, llmSvc.DeepCopy(), false, nil)
	require.NoError(t, err)
	assert.Equal(t, readyReplicas, llmSvc.Status.Replicas)
	assert.True(t, llmSvc.Status.ModelReady)
}

// TestCleanupResources_NoSPIRE does not error when SPIRE not configured.
func TestCleanupResources_NoSPIRE(t *testing.T) {
	s := buildLLMScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := setupReconciler(cl, s)

	llmSvc := makeLLMInferenceService("my-llm", "default")
	err := r.cleanupResources(context.Background(), llmSvc)
	require.NoError(t, err)
}

// TestVolumesEqual_SameMounts returns true for identical volume mounts.
func TestVolumesEqual_SameMounts(t *testing.T) {
	mounts := []corev1.VolumeMount{
		{Name: "model-store", MountPath: "/mnt/models"},
	}
	assert.True(t, reconciler.VolumeMountsEqual(mounts, mounts))
}

// TestVolumesEqual_DifferentMounts returns false for different volume mounts.
func TestVolumesEqual_DifferentMounts(t *testing.T) {
	mounts1 := []corev1.VolumeMount{{Name: "vol1", MountPath: "/a"}}
	mounts2 := []corev1.VolumeMount{{Name: "vol1", MountPath: "/b"}}
	assert.False(t, reconciler.VolumeMountsEqual(mounts1, mounts2))
}

// TestContainersEqual_SameContainers returns true.
func TestContainersEqual_SameContainers(t *testing.T) {
	containers := []corev1.Container{{Name: "vllm", Image: "vllm:latest"}}
	assert.True(t, reconciler.ContainersEqual(containers, containers))
}

// TestContainersEqual_DifferentImages returns false.
func TestContainersEqual_DifferentImages(t *testing.T) {
	c1 := []corev1.Container{{Name: "vllm", Image: "vllm:v1"}}
	c2 := []corev1.Container{{Name: "vllm", Image: "vllm:v2"}}
	assert.False(t, reconciler.ContainersEqual(c1, c2))
}

// TestPtrToHostPath returns a valid HostPathType pointer.
func TestPtrToHostPath(t *testing.T) {
	hp := deployment.PtrToHostPath(corev1.HostPathDirectory)
	require.NotNil(t, hp)
	assert.Equal(t, corev1.HostPathDirectory, *hp)
}

// TestMapLocalModelCacheToInferenceServices_MatchingModel returns requests for
// LLMInferenceServices that use the same model URI as the cache.
func TestMapLocalModelCacheToInferenceServices_MatchingModel(t *testing.T) {
	s := buildLLMScheme(t)

	llmSvc1 := makeLLMInferenceService("svc1", "default")
	llmSvc1.Spec.Model.URI = "hf://org/model-a"

	llmSvc2 := makeLLMInferenceService("svc2", "default")
	llmSvc2.Spec.Model.URI = "hf://org/model-b" // different model

	lmc := &servingv1alpha2.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-a", Namespace: "default"},
		Spec:       servingv1alpha2.LocalModelCacheSpec{SourceModelURI: "hf://org/model-a"},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc1, llmSvc2, lmc).Build()
	r := setupReconciler(cl, s)

	requests := r.mapLocalModelCacheToInferenceServices(context.Background(), lmc)
	require.Len(t, requests, 1)
	assert.Equal(t, "svc1", requests[0].Name)
}

// TestReconcileService_UpdatesExisting exercises service update path.
func TestReconcileService_UpdatesExisting(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("my-llm", "default")

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "old-port", Port: 9999},
			},
			Selector: map[string]string{"old-label": "val"},
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(llmSvc, existingSvc).Build()
	r := setupReconciler(cl, s)

	err := r.ServiceReconciler.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	var svc corev1.Service
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "my-llm", Namespace: "default",
	}, &svc))
	// Port should have been updated to the managed port.
	assert.Equal(t, "http-inference", svc.Spec.Ports[0].Name)
}

// TestReconcile_OCI_PullFailureStatus verifies Chaos path for OCI failures.
func TestReconcile_OCI_PullFailureStatus(t *testing.T) {
	s := buildLLMScheme(t)
	llmSvc := makeLLMInferenceService("fail-svc", "default")
	llmSvc.Spec.Model.URI = "oci://broken-registry/broken-model"

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(llmSvc).
		WithStatusSubresource(llmSvc).
		Build()
	r := setupReconciler(cl, s)

	// Simulate reconciliation loop
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{Name: "fail-svc", Namespace: "default"},
	})
	require.NoError(t, err)

	// Verify conditions even before successful rollout
	var updated servingv1alpha2.LLMInferenceService
	require.NoError(t, cl.Get(context.Background(), k8stypes.NamespacedName{
		Name: "fail-svc", Namespace: "default",
	}, &updated))

	// Deployment should be created but not ready
	foundDeploymentReady := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == servingv1alpha2.ConditionDeploymentReady {
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			foundDeploymentReady = true
		}
	}
	assert.True(t, foundDeploymentReady)
}
