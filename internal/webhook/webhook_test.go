/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package webhook_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/webhook"
)

// minimalValidSvc returns a LLMInferenceService that passes all validations.
// Tests can override individual fields to inject specific failure modes.
func minimalValidSvc() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: "llama3",
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "vllm", Image: "vllm/vllm-openai:latest"},
					},
				},
			},
		},
	}
}

// ---- Validating Webhook -------------------------------------------------------

func TestValidator_ValidateCreate_HappyPath(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{}
	warnings, err := v.ValidateCreate(context.Background(), minimalValidSvc())
	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestValidator_ValidateCreate_MissingURI(t *testing.T) {
	svc := minimalValidSvc()
	svc.Spec.Model.URI = ""
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.model.uri is required")
}

func TestValidator_ValidateCreate_MissingModelName(t *testing.T) {
	svc := minimalValidSvc()
	svc.Spec.Model.Name = ""
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.model.name is required")
}

func TestValidator_ValidateCreate_NoContainers(t *testing.T) {
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers = nil
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.template.spec.containers must have at least one container")
}

func TestValidator_ValidateCreate_UnknownScheme(t *testing.T) {
	for _, uri := range []string{
		"ftp://some-host/model",
		"huggingface://org/model",
	} {
		svc := minimalValidSvc()
		svc.Spec.Model.URI = uri
		v := &webhook.LLMInferenceServiceValidator{}
		_, err := v.ValidateCreate(context.Background(), svc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.model.uri must start with one of")
	}
}

func TestValidator_ValidateCreate_ValidSchemes(t *testing.T) {
	schemes := []string{
		"hf://meta-llama/Llama-3.2-1B",
		"hf-mirror://internal.corp/llama",
		"s3://bucket/prefix/model",
		"gs://bucket/model",
		"pvc://my-pvc/model",
		"oci://registry.corp/model:tag",
		"ocis://registry.corp/model:tag",
		"seaweedfs://seaweedfs.corp/model",
		"http://internal/model",
		"https://internal/model",
	}
	v := &webhook.LLMInferenceServiceValidator{}
	for _, uri := range schemes {
		t.Run(uri, func(t *testing.T) {
			svc := minimalValidSvc()
			svc.Spec.Model.URI = uri
			_, err := v.ValidateCreate(context.Background(), svc)
			assert.NoError(t, err, "URI %q should be accepted", uri)
		})
	}
}

// FedRAMP mode rejects hf:// but accepts other schemes.
func TestValidator_ValidateCreate_FedRAMPMode_RejectsHF(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{FedRAMPMode: true}
	svc := minimalValidSvc()
	svc.Spec.Model.URI = "hf://meta-llama/Llama-3.2-1B"
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hf:// URIs are not permitted in FedRAMP mode")
}

func TestValidator_ValidateCreate_FedRAMPMode_AcceptsOCI(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{FedRAMPMode: true}
	svc := minimalValidSvc()
	svc.Spec.Model.URI = "oci://registry.corp/llama3:latest"
	_, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
}

func TestValidator_ValidateCreate_FedRAMPMode_AcceptsOCIS(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{FedRAMPMode: true}
	svc := minimalValidSvc()
	svc.Spec.Model.URI = "ocis://registry.corp/llama3:latest"
	_, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
}

func TestValidator_ValidateCreate_FedRAMPMode_AcceptsS3(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{FedRAMPMode: true}
	svc := minimalValidSvc()
	svc.Spec.Model.URI = "s3://my-gov-bucket/llama3"
	_, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
}

// Tensor parallelism > 1 with no GPU limit should produce a warning, not an error.
func TestValidator_ValidateCreate_TensorParallelism_NoGPU_Warning(t *testing.T) {
	svc := minimalValidSvc()
	tp := int32(4)
	svc.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Tensor: &tp}
	// Explicitly no GPU resources
	svc.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{}

	v := &webhook.LLMInferenceServiceValidator{}
	warnings, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err, "missing GPU with tensor parallelism should be a warning, not a hard error")
	require.NotEmpty(t, warnings)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "tensor parallelism") {
			found = true
		}
	}
	assert.True(t, found, "expected tensor parallelism GPU warning, got: %v", warnings)
}

// Tensor parallelism = 1 with no GPU limit: no warning.
func TestValidator_ValidateCreate_TensorParallelism_One_NoWarning(t *testing.T) {
	svc := minimalValidSvc()
	tp := int32(1)
	svc.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Tensor: &tp}
	v := &webhook.LLMInferenceServiceValidator{}
	warnings, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

// Tensor parallelism > 1 with GPU limit set: no warning.
func TestValidator_ValidateCreate_TensorParallelism_WithGPU_NoWarning(t *testing.T) {
	svc := minimalValidSvc()
	tp := int32(4)
	svc.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{Tensor: &tp}
	svc.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			"nvidia.com/gpu": resource.MustParse("4"),
		},
	}
	v := &webhook.LLMInferenceServiceValidator{}
	warnings, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

// minReplicas > maxReplicas is a hard error.
func TestValidator_ValidateCreate_Scaling_MinGTMax(t *testing.T) {
	svc := minimalValidSvc()
	min, max := int32(5), int32(2)
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: &min,
		MaxReplicas: &max,
	}
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.scaling.minReplicas must be <= spec.scaling.maxReplicas")
}

func TestValidator_ValidateCreate_Scaling_MinLEMax_Valid(t *testing.T) {
	svc := minimalValidSvc()
	min, max := int32(2), int32(10)
	svc.Spec.Scaling = &servingv1alpha2.ScalingSpec{
		MinReplicas: &min,
		MaxReplicas: &max,
	}
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	assert.NoError(t, err)
}

func TestValidator_ValidateCreate_Prefill_NoContainers(t *testing.T) {
	svc := minimalValidSvc()
	svc.Spec.Prefill = &servingv1alpha2.PrefillSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: nil},
		},
	}
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.prefill.template.spec.containers must have at least one container")
}

// ValidateUpdate delegates to the same logic as ValidateCreate.
func TestValidator_ValidateUpdate_SameRulesAsCreate(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{}
	old := minimalValidSvc()
	newSvc := minimalValidSvc()
	newSvc.Spec.Model.URI = "" // inject error

	_, err := v.ValidateUpdate(context.Background(), old, newSvc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.model.uri is required")
}

// ValidateDelete is always a no-op.
func TestValidator_ValidateDelete_NoOp(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{}
	warnings, err := v.ValidateDelete(context.Background(), minimalValidSvc())
	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

// Multiple errors are all reported in a single error (not just the first).
func TestValidator_ValidateCreate_MultipleErrors(t *testing.T) {
	svc := minimalValidSvc()
	svc.Spec.Model.URI = ""
	svc.Spec.Model.Name = ""
	svc.Spec.Template.Spec.Containers = nil
	v := &webhook.LLMInferenceServiceValidator{}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.model.uri is required")
	assert.Contains(t, err.Error(), "spec.model.name is required")
	assert.Contains(t, err.Error(), "spec.template.spec.containers must have at least one container")
}

// ---- Mutating Webhook (Defaulter) --------------------------------------------

func TestDefaulter_Default_HFMirrorRewrite(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{HFMirrorURL: "https://hf-mirror.corp.internal"}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers = []corev1.Container{{Name: "vllm", Image: "vllm/vllm-openai:latest"}}
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, "hf-mirror://meta-llama/Llama-3.2-1B", svc.Spec.Model.URI,
		"hf:// should be rewritten to hf-mirror://")
}

func TestDefaulter_Default_NoMirrorURL_NoRewrite(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{HFMirrorURL: ""}
	svc := minimalValidSvc()
	original := svc.Spec.Model.URI
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, original, svc.Spec.Model.URI, "URI should not change when no mirror configured")
}

func TestDefaulter_Default_NonHFURI_NotRewritten(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{HFMirrorURL: "https://hf-mirror.corp.internal"}
	svc := minimalValidSvc()
	svc.Spec.Model.URI = "s3://my-bucket/model"
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, "s3://my-bucket/model", svc.Spec.Model.URI, "non-hf:// URIs must not be rewritten")
}

func TestDefaulter_Default_ReplicasDefaultedToOne(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Replicas = nil
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	require.NotNil(t, svc.Spec.Replicas)
	assert.Equal(t, int32(1), *svc.Spec.Replicas)
}

func TestDefaulter_Default_ExistingReplicasNotOverwritten(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	three := int32(3)
	svc.Spec.Replicas = &three
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Equal(t, int32(3), *svc.Spec.Replicas)
}

func TestDefaulter_Default_EmptyImageRemainsUnsetForController(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers[0].Image = ""
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Empty(t, svc.Spec.Template.Spec.Containers[0].Image,
		"controller must resolve the configured runtime image")
}

func TestDefaulter_Default_SecurityContextInjected(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers[0].SecurityContext = nil
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	sc := svc.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	// RunAsNonRoot IS set by the webhook for security hardening
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot, "webhook must default RunAsNonRoot to true")
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
}

func TestDefaulter_Default_ExistingSecurityContextPreserved(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	f := false
	svc.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		RunAsNonRoot: &f, // explicit false — should NOT be overwritten
	}
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.False(t, *svc.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot,
		"existing RunAsNonRoot=false must not be overwritten by defaulter")
}

func TestDefaulter_Default_DefaultPortsInjected(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers[0].Ports = nil
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	ports := svc.Spec.Template.Spec.Containers[0].Ports
	require.Len(t, ports, 2)
	names := make(map[string]int32, 2)
	for _, p := range ports {
		names[p.Name] = p.ContainerPort
	}
	assert.Equal(t, int32(8000), names["http"])
	assert.Equal(t, int32(8001), names["grpc"])
}

func TestDefaulter_Default_ExistingPortsNotOverwritten(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
		{Name: "custom", ContainerPort: 9999},
	}
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	assert.Len(t, svc.Spec.Template.Spec.Containers[0].Ports, 1,
		"existing ports must not be overwritten by defaulter")
	assert.Equal(t, int32(9999), svc.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
}

func TestDefaulter_Default_SchedulerReplicasDefaultedToOne(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
	svc.Spec.Router.Scheduler.Replicas = nil
	err := d.Default(context.Background(), svc)
	require.NoError(t, err)
	require.NotNil(t, svc.Spec.Router.Scheduler.Replicas)
	assert.Equal(t, int32(1), *svc.Spec.Router.Scheduler.Replicas)
}

func TestDefaulter_Default_OmittedSchedulerRemainsDisabled(t *testing.T) {
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Router.Scheduler = nil
	require.NoError(t, d.Default(context.Background(), svc))
	assert.Nil(t, svc.Spec.Router.Scheduler)
}

func TestValidator_ModelRevisionRules(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{}
	valid := minimalValidSvc()
	valid.Spec.Model.Revision = "refs/pr/42"
	_, err := v.ValidateCreate(context.Background(), valid)
	require.NoError(t, err)

	nonHF := minimalValidSvc()
	nonHF.Spec.Model.URI = "s3://bucket/model"
	nonHF.Spec.Model.Revision = "v1"
	_, err = v.ValidateCreate(context.Background(), nonHF)
	require.ErrorContains(t, err, "supported only")

	ambiguous := minimalValidSvc()
	ambiguous.Spec.Model.URI = "hf://org/model@main"
	ambiguous.Spec.Model.Revision = "v1"
	_, err = v.ValidateCreate(context.Background(), ambiguous)
	require.ErrorContains(t, err, "cannot be combined")
}

func TestValidator_TypedLMCacheRules(t *testing.T) {
	v := &webhook.LLMInferenceServiceValidator{}
	svc := minimalValidSvc()
	svc.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{
		Connector: "lmcache", LMCache: &servingv1alpha2.LMCacheSpec{Mode: servingv1alpha2.LMCacheModeMultiprocess},
	}}
	_, err := v.ValidateCreate(context.Background(), svc)
	require.ErrorContains(t, err, "engineRef.name is required")

	svc.Spec.KVCache.Transfer.LMCache.EngineRef = &corev1.LocalObjectReference{Name: "shared-kv"}
	_, err = v.ValidateCreate(context.Background(), svc)
	require.NoError(t, err)

	svc.Spec.KVCache.Transfer.Connector = "nixl"
	_, err = v.ValidateCreate(context.Background(), svc)
	require.ErrorContains(t, err, "requires connector=lmcache")
}

func TestDefaulter_Default_NoContainers_NoOp(t *testing.T) {
	// When there are no containers the defaulter must not panic.
	d := &webhook.LLMInferenceServiceDefaulter{}
	svc := minimalValidSvc()
	svc.Spec.Template.Spec.Containers = nil
	err := d.Default(context.Background(), svc)
	assert.NoError(t, err)
}
