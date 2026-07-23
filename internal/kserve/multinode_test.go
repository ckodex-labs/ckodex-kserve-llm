package kserve

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func int32Ptr(value int32) *int32 { return &value }

func multiNodeService() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1alpha2.GroupVersion,
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gemma",
			Namespace: "models",
			UID:       types.UID("gemma-uid"),
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "unsloth/gemma-4-26B-A4B-it-NVFP4",
				URI:  "pvc://gemma4-weights",
				Storage: &servingv1alpha2.StorageSpec{
					ServiceAccountName: "model-reader",
				},
			},
			Replicas: int32Ptr(1),
			Parallelism: &servingv1alpha2.ParallelismSpec{
				Tensor:   int32Ptr(2),
				Pipeline: int32Ptr(2),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"topology": "distributed"},
					Annotations: map[string]string{"example.com/head": "enabled"},
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"accelerator": "blackwell"},
					Containers: []corev1.Container{{
						Name: "model",
						Env:  []corev1.EnvVar{{Name: "HF_HOME", Value: "/cache"}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
							},
						},
					}},
				},
			},
			Worker: &servingv1alpha2.WorkerSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"example.com/worker": "enabled"},
					},
					Spec: corev1.PodSpec{
						NodeSelector: map[string]string{"accelerator": "blackwell"},
						Containers: []corev1.Container{{
							Name: "worker",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
								},
							},
						}},
					},
				},
			},
		},
	}
}

func TestBuildMapsKServeV019MultiNodeContract(t *testing.T) {
	obj, err := (&Reconciler{RuntimeName: "custom-multinode-runtime"}).Build(multiNodeService())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got := obj.GetAnnotations()[deploymentModeAnnotation]; got != "Standard" {
		t.Fatalf("deployment mode = %q, want Standard", got)
	}
	if got := obj.GetAnnotations()[autoscalerAnnotation]; got != "none" {
		t.Fatalf("autoscaler class = %q, want none", got)
	}

	predictor, _, _ := unstructuredMap(obj.Object, "spec", "predictor")
	model, _, _ := unstructuredMap(predictor, "model")
	if model["runtime"] != "custom-multinode-runtime" {
		t.Fatalf("runtime = %v", model["runtime"])
	}
	if model["storageUri"] != "pvc://gemma4-weights" {
		t.Fatalf("storageUri = %v", model["storageUri"])
	}
	if model["image"] != nil {
		t.Fatal("model image override must not replace the KServe runtime image")
	}
	if _, found := model["env"]; !found {
		t.Fatal("head container environment override was not propagated")
	}
	assertGPUResources(t, model["resources"], "head")
	headNodeSelector := predictor["nodeSelector"].(map[string]interface{})
	if headNodeSelector["accelerator"] != "blackwell" {
		t.Fatalf("head node selector = %#v", headNodeSelector)
	}
	if predictor["serviceAccountName"] != "model-reader" {
		t.Fatalf("serviceAccountName = %v", predictor["serviceAccountName"])
	}
	predictorAnnotations := predictor["annotations"].(map[string]interface{})
	if predictorAnnotations["example.com/head"] != "enabled" ||
		predictorAnnotations["example.com/worker"] != "enabled" {
		t.Fatalf("predictor annotations = %#v", predictorAnnotations)
	}

	worker, _, _ := unstructuredMap(predictor, "workerSpec")
	if worker["pipelineParallelSize"] != int64(2) || worker["tensorParallelSize"] != int64(2) {
		t.Fatalf("worker parallelism = %#v", worker)
	}
	containers, ok := worker["containers"].([]interface{})
	if !ok || len(containers) != 1 {
		t.Fatalf("worker containers = %#v", worker["containers"])
	}
	container := containers[0].(map[string]interface{})
	if container["name"] != "worker-container" {
		t.Fatalf("worker container name = %v", container["name"])
	}
	if _, found := container["image"]; found {
		t.Fatal("worker image override must not replace the KServe runtime image")
	}
	assertGPUResources(t, container["resources"], "worker")
	workerNodeSelector := worker["nodeSelector"].(map[string]interface{})
	if workerNodeSelector["accelerator"] != "blackwell" {
		t.Fatalf("worker node selector = %#v", workerNodeSelector)
	}
}

func TestBuildRejectsUnsupportedMultiNodeInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*servingv1alpha2.LLMInferenceService)
	}{
		{"hf storage", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Model.URI = "hf://org/model" }},
		{"oci storage", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Model.URI = "oci://registry/model" }},
		{"scaled heads", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Replicas = int32Ptr(2) }},
		{"autoscaling", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Scaling = &servingv1alpha2.ScalingSpec{} }},
		{"data parallel", func(s *servingv1alpha2.LLMInferenceService) { s.Spec.Parallelism.Data = int32Ptr(2) }},
		{"runtime image override", func(s *servingv1alpha2.LLMInferenceService) {
			s.Spec.Worker.Template.Spec.Containers[0].Image = "vllm/vllm-openai:v0.25.1"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := multiNodeService()
			tt.mutate(svc)
			if _, err := (&Reconciler{}).Build(svc); err == nil {
				t.Fatal("Build() error = nil, want validation error")
			}
		})
	}
}

func TestRequiresMultiNodeDoesNotCaptureSingleNodeTensorParallelism(t *testing.T) {
	svc := multiNodeService()
	svc.Spec.Worker = nil
	svc.Spec.Parallelism.Pipeline = nil
	if RequiresMultiNode(svc) {
		t.Fatal("tensor parallelism alone must remain on the single-node path")
	}
}

func TestReconcileCreatesInferenceServiceAndRemovesOwnedLegacyDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := autoscalingv2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	svc := multiNodeService()
	legacy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: svc.APIVersion,
				Kind:       svc.Kind,
				Name:       svc.Name,
				UID:        svc.UID,
				Controller: boolPtr(true),
			}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, legacy, sharedModelPVC(svc)).Build()
	reconciler := &Reconciler{Client: cl, Scheme: scheme}

	if err := reconciler.Reconcile(context.Background(), svc); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	isvc := NewInferenceService()
	if err := cl.Get(context.Background(), types.NamespacedName{
		Name: svc.Name, Namespace: svc.Namespace,
	}, isvc); err != nil {
		t.Fatalf("KServe InferenceService not created: %v", err)
	}
	if err := cl.Get(context.Background(), types.NamespacedName{
		Name: svc.Name, Namespace: svc.Namespace,
	}, &appsv1.Deployment{}); err == nil {
		t.Fatal("owned legacy Deployment was not removed")
	}
}

func TestReconcilePreservesExternalMetadataAndDoesNotRewriteUnchangedObject(t *testing.T) {
	scheme := multiNodeScheme(t)
	svc := multiNodeService()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, sharedModelPVC(svc)).Build()
	reconciler := &Reconciler{Client: cl, Scheme: scheme}
	ctx := context.Background()

	if err := reconciler.Reconcile(ctx, svc); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	isvc := NewInferenceService()
	key := types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}
	if err := cl.Get(ctx, key, isvc); err != nil {
		t.Fatal(err)
	}
	annotations := isvc.GetAnnotations()
	annotations["external.example.com/state"] = "preserve"
	isvc.SetAnnotations(annotations)
	if err := cl.Update(ctx, isvc); err != nil {
		t.Fatal(err)
	}
	resourceVersion := isvc.GetResourceVersion()

	if err := reconciler.Reconcile(ctx, svc); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if err := cl.Get(ctx, key, isvc); err != nil {
		t.Fatal(err)
	}
	if isvc.GetAnnotations()["external.example.com/state"] != "preserve" {
		t.Fatal("external annotation was removed")
	}
	if isvc.GetResourceVersion() != resourceVersion {
		t.Fatalf("unchanged object was rewritten: resourceVersion %s -> %s",
			resourceVersion, isvc.GetResourceVersion())
	}
}

func TestReconcileToleratesMissingLegacyLeaderWorkerSetCRD(t *testing.T) {
	scheme := multiNodeScheme(t)
	svc := multiNodeService()
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, sharedModelPVC(svc)).Build()
	reconciler := &Reconciler{
		Client: &missingLWSClient{Client: base},
		Scheme: scheme,
	}
	if err := reconciler.Reconcile(context.Background(), svc); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestReconcileRejectsModelPVCWithoutReadWriteMany(t *testing.T) {
	scheme := multiNodeScheme(t)
	svc := multiNodeService()
	pvc := sharedModelPVC(svc)
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, pvc).Build()
	reconciler := &Reconciler{Client: cl, Scheme: scheme}

	err := reconciler.Reconcile(context.Background(), svc)
	if err == nil || !strings.Contains(err.Error(), "must declare ReadWriteMany") {
		t.Fatalf("Reconcile() error = %v, want ReadWriteMany validation error", err)
	}
	isvc := NewInferenceService()
	if err := cl.Get(context.Background(), types.NamespacedName{
		Name: svc.Name, Namespace: svc.Namespace,
	}, isvc); !apierrors.IsNotFound(err) {
		t.Fatalf("InferenceService created for non-RWX PVC: %v", err)
	}
}

func TestModelPVCClaimNamePreservesKServeSubpath(t *testing.T) {
	claimName, err := modelPVCClaimName("pvc://gemma4-weights/models/gemma")
	if err != nil {
		t.Fatalf("modelPVCClaimName() error = %v", err)
	}
	if claimName != "gemma4-weights" {
		t.Fatalf("claim name = %q, want gemma4-weights", claimName)
	}

	for _, uri := range []string{"oci://registry/model", "pvc://"} {
		if _, err := modelPVCClaimName(uri); err == nil {
			t.Fatalf("modelPVCClaimName(%q) error = nil, want validation error", uri)
		}
	}
}

func sharedModelPVC(svc *servingv1alpha2.LLMInferenceService) *corev1.PersistentVolumeClaim {
	claimName, err := modelPVCClaimName(svc.Spec.Model.URI)
	if err != nil {
		panic(err)
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: svc.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}
}

func assertGPUResources(t *testing.T, raw interface{}, location string) {
	t.Helper()
	resources, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("%s resources = %#v", location, raw)
	}
	for _, field := range []string{"requests", "limits"} {
		values, ok := resources[field].(map[string]interface{})
		if !ok || values["nvidia.com/gpu"] != "2" {
			t.Fatalf("%s GPU %s = %#v", location, field, resources[field])
		}
	}
}

func multiNodeScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := autoscalingv2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

type missingLWSClient struct {
	client.Client
}

func (c *missingLWSClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if unstructured, ok := obj.(*unstructured.Unstructured); ok &&
		unstructured.GroupVersionKind().Group == "leaderworkerset.x-k8s.io" {
		return &meta.NoKindMatchError{
			GroupKind: schema.GroupKind{Group: "leaderworkerset.x-k8s.io", Kind: "LeaderWorkerSet"},
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func unstructuredMap(object map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	current := object
	for _, field := range fields {
		next, ok := current[field].(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current = next
	}
	return current, true, nil
}

func boolPtr(value bool) *bool { return &value }
