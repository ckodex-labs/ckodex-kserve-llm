/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ---- scheme helpers --------------------------------------------------------

func secScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, networkingv1.AddToScheme(s))
	return s
}

func minimalLLMSvc(name, ns string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
		},
	}
}

// ---- DefaultVaultConfig ----------------------------------------------------

func TestDefaultVaultConfig_Address(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "http://vault.vault:8200", cfg.Address)
}

func TestDefaultVaultConfig_Role(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "ckodex-kserve-llm", cfg.Role)
}

func TestDefaultVaultConfig_SecretPath(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "secret/data/models", cfg.SecretPath)
}

func TestDefaultVaultConfig_AuthMethod_Kubernetes(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Equal(t, "kubernetes", cfg.AuthMethod)
}

// CRITICAL: TLSSkipVerify must be false by default.
// The annotation "vault.hashicorp.com/tls-skip-verify" must NEVER be injected
// unless the operator is explicitly configured for dev mode.
func TestDefaultVaultConfig_TLSSkipVerify_False(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.False(t, cfg.TLSSkipVerify, "TLSSkipVerify must default to false — disabling TLS is dev-only and must never reach production")
}

// CRITICAL: TransitKeyName must be empty by default.
// The transit annotation must not be injected unless explicitly configured.
func TestDefaultVaultConfig_TransitKeyName_Empty(t *testing.T) {
	cfg := DefaultVaultConfig()
	assert.Empty(t, cfg.TransitKeyName, "TransitKeyName must default to empty — transit encryption is opt-in")
}

// ---- buildAnnotations — core inject ----------------------------------------

func TestBuildAnnotations_CoreInjectPresent(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	svc := minimalLLMSvc("llama3", "default")

	ann := v.buildAnnotations(svc)

	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-inject"])
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-init-first"])
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-pre-populate-only"])
	assert.Equal(t, "ckodex-kserve-llm", ann["vault.hashicorp.com/role"])
	assert.Equal(t, "1000", ann["vault.hashicorp.com/agent-run-as-user"])
	assert.Equal(t, "1000", ann["vault.hashicorp.com/agent-run-as-group"])
}

func TestBuildAnnotations_ResourceLimits(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	ann := v.buildAnnotations(minimalLLMSvc("phi3", "default"))

	assert.Equal(t, "100m", ann["vault.hashicorp.com/agent-limits-cpu"])
	assert.Equal(t, "64Mi", ann["vault.hashicorp.com/agent-limits-mem"])
	assert.Equal(t, "50m", ann["vault.hashicorp.com/agent-requests-cpu"])
	assert.Equal(t, "32Mi", ann["vault.hashicorp.com/agent-requests-mem"])
}

// ---- buildAnnotations — security fallbacks (absence tests) -----------------

// CRITICAL: TLSSkipVerify=false must produce NO tls-skip-verify annotation.
// An accidental "false" string annotation could still alter vault agent behaviour.
func TestBuildAnnotations_TLSSkipVerifyFalse_NoAnnotation(t *testing.T) {
	cfg := DefaultVaultConfig()
	// Default: TLSSkipVerify=false
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("secure", "prod"))

	_, present := ann["vault.hashicorp.com/tls-skip-verify"]
	assert.False(t, present, "tls-skip-verify annotation must be absent when TLSSkipVerify=false")
}

// Verify that when TLSSkipVerify is explicitly enabled the annotation IS injected.
func TestBuildAnnotations_TLSSkipVerifyTrue_AnnotationPresent(t *testing.T) {
	cfg := DefaultVaultConfig()
	cfg.TLSSkipVerify = true
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("dev", "dev"))

	assert.Equal(t, "true", ann["vault.hashicorp.com/tls-skip-verify"])
}

// CRITICAL: Empty TransitKeyName must produce NO transit annotation.
func TestBuildAnnotations_TransitKeyNameEmpty_NoAnnotation(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}

	ann := v.buildAnnotations(minimalLLMSvc("secure", "prod"))

	_, present := ann["vault.hashicorp.com/agent-inject-secret-transit-key"]
	assert.False(t, present, "transit-key annotation must be absent when TransitKeyName is empty")
}

func TestBuildAnnotations_TransitKeyNameSet_AnnotationPresent(t *testing.T) {
	cfg := DefaultVaultConfig()
	cfg.TransitKeyName = "model-encryption-key"
	v := &VaultReconciler{Config: cfg}

	ann := v.buildAnnotations(minimalLLMSvc("encrypted", "prod"))

	val, present := ann["vault.hashicorp.com/agent-inject-secret-transit-key"]
	require.True(t, present, "transit annotation must be present when TransitKeyName is set")
	assert.Equal(t, "transit/keys/model-encryption-key", val)
}

func TestBuildAnnotations_SecretPathContainsNamespaceAndName(t *testing.T) {
	v := &VaultReconciler{Config: DefaultVaultConfig()}
	svc := minimalLLMSvc("phi3", "staging")

	ann := v.buildAnnotations(svc)

	// Model-scoped path must include namespace and name for isolation.
	hfSecret := ann["vault.hashicorp.com/agent-inject-secret-hf-token"]
	assert.Contains(t, hfSecret, "staging")
	assert.Contains(t, hfSecret, "phi3")
}

// ---- ReconcileVault --------------------------------------------------------

func TestReconcileVault_DeploymentNotFound_NoError(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	// Deployment does not exist — must return nil (no-op, deployment not yet created)
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
}

func TestReconcileVault_AnnotatesDeployment(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "llama3", Namespace: "default"},
	}

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, dep).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	require.NoError(t, v.ReconcileVault(context.Background(), svc))

	var updated appsv1.Deployment
	require.NoError(t, v.Get(context.Background(),
		types.NamespacedName{Name: "llama3", Namespace: "default"}, &updated))

	ann := updated.Spec.Template.Annotations
	require.NotNil(t, ann)
	assert.Equal(t, "true", ann["vault.hashicorp.com/agent-inject"])
}

func TestReconcileVault_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "llama3", Namespace: "default"},
	}

	v := &VaultReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, dep).Build(),
		Scheme: scheme,
		Config: DefaultVaultConfig(),
	}

	// Apply twice — both calls must succeed without error
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
	require.NoError(t, v.ReconcileVault(context.Background(), svc))
}

// ---- DefaultOPAConfig ------------------------------------------------------

func TestDefaultOPAConfig_MaxGPUs(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Equal(t, int64(8), cfg.MaxGPUsPerNamespace)
}

func TestDefaultOPAConfig_MaxReplicas(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Equal(t, int64(16), cfg.MaxReplicasPerService)
}

func TestDefaultOPAConfig_AllowedRegistries_Count(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Len(t, cfg.AllowedRegistries, 4)
}

func TestDefaultOPAConfig_AllowedRegistries_Values(t *testing.T) {
	cfg := DefaultOPAConfig()
	assert.Contains(t, cfg.AllowedRegistries, "ghcr.io/ckodex/")
	assert.Contains(t, cfg.AllowedRegistries, "vllm/")
	assert.Contains(t, cfg.AllowedRegistries, "kserve/")
	assert.Contains(t, cfg.AllowedRegistries, "gcr.io/distroless/")
}

// ---- buildConstraintTemplate -----------------------------------------------

func TestBuildConstraintTemplate_APIVersion(t *testing.T) {
	ct := buildConstraintTemplate("testpolicy", "package test\n", nil)
	assert.Equal(t, "templates.gatekeeper.sh/v1", ct.GetAPIVersion())
	assert.Equal(t, "ConstraintTemplate", ct.GetKind())
}

func TestBuildConstraintTemplate_NameAndLabel(t *testing.T) {
	ct := buildConstraintTemplate("llmresourcequota", "package x\n", nil)
	assert.Equal(t, "llmresourcequota", ct.GetName())
	labels := ct.GetLabels()
	assert.Equal(t, "ckodex-kserve-llm-operator", labels["app.kubernetes.io/managed-by"])
}

func TestBuildConstraintTemplate_RegoPresent(t *testing.T) {
	rego := "package mypkg\nviolation[{}] { true }"
	ct := buildConstraintTemplate("mypolicy", rego, nil)

	targets, _, _ := unstructured.NestedSlice(ct.Object, "spec", "targets")
	require.Len(t, targets, 1)
	target := targets[0].(map[string]interface{})
	assert.Equal(t, rego, target["rego"])
}

func TestBuildConstraintTemplate_WithParams_ValidationPresent(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "maxReplicas", "type": "integer"},
	}
	ct := buildConstraintTemplate("quotapolicy", "package q\n", params)

	_, found, err := unstructured.NestedMap(ct.Object, "spec", "crd", "spec", "validation", "openAPIV3Schema", "properties")
	require.NoError(t, err)
	assert.True(t, found, "params should produce openAPIV3Schema properties")
}

func TestBuildConstraintTemplate_NoParams_NoValidation(t *testing.T) {
	ct := buildConstraintTemplate("noparam", "package x\n", nil)

	_, found, _ := unstructured.NestedMap(ct.Object, "spec", "crd", "spec", "validation")
	assert.False(t, found, "nil params must produce no validation section")
}

// ---- buildConstraint -------------------------------------------------------

func TestBuildConstraint_APIVersion(t *testing.T) {
	c := buildConstraint("llmresourcequota", "llm-rq", "default", nil)
	assert.Equal(t, "constraints.gatekeeper.sh/v1beta1", c.GetAPIVersion())
	assert.Equal(t, "llmresourcequota", c.GetKind())
}

func TestBuildConstraint_NamespaceInMatch(t *testing.T) {
	c := buildConstraint("mykind", "my-constraint", "prod", nil)
	namespaces, _, _ := unstructured.NestedSlice(c.Object, "spec", "match", "namespaces")
	require.Len(t, namespaces, 1)
	assert.Equal(t, "prod", namespaces[0])
}

func TestBuildConstraint_KindMatchesAPIGroup(t *testing.T) {
	c := buildConstraint("llmresourcequota", "rq", "default", nil)
	kinds, _, _ := unstructured.NestedSlice(c.Object, "spec", "match", "kinds")
	require.Len(t, kinds, 1)
	k := kinds[0].(map[string]interface{})
	groups := k["apiGroups"].([]interface{})
	assert.Equal(t, "serving.ckodex.com", groups[0])
}

func TestBuildConstraint_WithParams(t *testing.T) {
	c := buildConstraint("mykind", "my-c", "ns", map[string]interface{}{"maxGPUs": int64(8)})
	params, found, _ := unstructured.NestedMap(c.Object, "spec", "parameters")
	require.True(t, found)
	assert.Equal(t, int64(8), params["maxGPUs"])
}

func TestBuildConstraint_NilParams_NoParameters(t *testing.T) {
	c := buildConstraint("mykind", "my-c", "ns", nil)
	_, found, _ := unstructured.NestedMap(c.Object, "spec", "parameters")
	assert.False(t, found, "nil params must produce no parameters field")
}

// ---- buildParamProperties --------------------------------------------------

func TestBuildParamProperties_IntegerType(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "maxReplicas", "type": "integer"},
	}
	props := buildParamProperties(params)
	p := props["maxReplicas"].(map[string]interface{})
	assert.Equal(t, "integer", p["type"])
}

func TestBuildParamProperties_ArrayWithItems(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "allowedRegistries", "type": "array", "items": map[string]interface{}{"type": "string"}},
	}
	props := buildParamProperties(params)
	p := props["allowedRegistries"].(map[string]interface{})
	assert.Equal(t, "array", p["type"])
	assert.NotNil(t, p["items"])
}

func TestBuildParamProperties_MultipleParams(t *testing.T) {
	params := []map[string]interface{}{
		{"name": "a", "type": "integer"},
		{"name": "b", "type": "string"},
	}
	props := buildParamProperties(params)
	assert.Len(t, props, 2)
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
}

// ---- OPAReconciler.ReconcileOPA --------------------------------------------

func TestReconcileOPA_CreatesAllConstraints(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))

	// Verify the resource-quota constraint was created (checks applyUnstructured path)
	var c unstructured.Unstructured
	c.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "constraints.gatekeeper.sh",
		Version: "v1beta1",
		Kind:    "llmresourcequota",
	})
	require.NoError(t, o.Get(context.Background(),
		types.NamespacedName{Name: "llm-resource-quota"}, &c))
}

func TestReconcileOPA_ImageAllowlistConstraintCreated(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "prod", DefaultOPAConfig()))

	var c unstructured.Unstructured
	c.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "constraints.gatekeeper.sh",
		Version: "v1beta1",
		Kind:    "llmimageallowlist",
	})
	require.NoError(t, o.Get(context.Background(),
		types.NamespacedName{Name: "llm-image-allowlist"}, &c))
}

func TestReconcileOPA_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	o := &OPAReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))
	require.NoError(t, o.ReconcileOPA(context.Background(), "default", DefaultOPAConfig()))
}

// ---- SPIFFEIDForService ----------------------------------------------------

func TestSPIFFEIDForService_Format(t *testing.T) {
	id := SPIFFEIDForService("prod", "vllm-sa", "llama3")
	assert.Equal(t, "spiffe://ckodex.com/ns/prod/sa/vllm-sa/model/llama3", id)
}

func TestSPIFFEIDForService_TrustDomain(t *testing.T) {
	id := SPIFFEIDForService("any", "sa", "model")
	assert.Contains(t, id, "spiffe://"+SPIFFETrustDomain+"/")
}

func TestSPIFFEIDForService_DifferentInputs(t *testing.T) {
	a := SPIFFEIDForService("ns1", "sa1", "m1")
	b := SPIFFEIDForService("ns2", "sa2", "m2")
	assert.NotEqual(t, a, b)
}

// ---- SPIREReconciler.ReconcileSPIRE ----------------------------------------

func TestReconcileSPIRE_CreatesStatefulSet(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))

	var ss appsv1.StatefulSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-server", Namespace: "spire"}, &ss))

	assert.Equal(t, SPIREServerImage, ss.Spec.Template.Spec.Containers[0].Image)
}

func TestReconcileSPIRE_CreatesDaemonSet(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))

	var ds appsv1.DaemonSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-agent", Namespace: "spire"}, &ds))

	assert.Equal(t, SPIREAgentImage, ds.Spec.Template.Spec.Containers[0].Image)
	assert.True(t, ds.Spec.Template.Spec.HostNetwork, "SPIRE agent requires HostNetwork for node attestation")
}

func TestReconcileSPIRE_ServerSecurityContext(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "default"))

	var ss appsv1.StatefulSet
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "spire-server", Namespace: "default"}, &ss))

	sc := ss.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.RunAsNonRoot)
	assert.False(t, *sc.AllowPrivilegeEscalation)
}

func TestReconcileSPIRE_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	r := &SPIREReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))
	require.NoError(t, r.ReconcileSPIRE(context.Background(), "spire"))
}

// ---- SPIREReconciler.InjectSidecar -----------------------------------------

func TestInjectSidecar_AppendsCSIVolume(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{}

	r.InjectSidecar(spec, svc)

	require.Len(t, spec.Volumes, 1)
	assert.Equal(t, "spiffe-workload-api", spec.Volumes[0].Name)
	require.NotNil(t, spec.Volumes[0].CSI)
	assert.Equal(t, "spiffe.csi.spiffe.io", spec.Volumes[0].CSI.Driver)
}

func TestInjectSidecar_AppendsSidecarContainer(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm", Image: "vllm/vllm:latest"}},
	}

	r.InjectSidecar(spec, svc)

	// Original container + sidecar = 2
	require.Len(t, spec.Containers, 2)
	sidecar := spec.Containers[1]
	assert.Equal(t, "spiffe-sidecar", sidecar.Name)
	assert.Equal(t, SPIFFEHelperImage, sidecar.Image)
}

// CRITICAL: ReadOnlyRootFilesystem must be false — the helper writes cert/key PEM files.
func TestInjectSidecar_SidecarReadOnlyRootFilesystem_False(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	require.NotNil(t, sidecar.SecurityContext)
	require.NotNil(t, sidecar.SecurityContext.ReadOnlyRootFilesystem)
	assert.False(t, *sidecar.SecurityContext.ReadOnlyRootFilesystem,
		"spiffe-helper writes PEM files to /run/spiffe/certs — ReadOnlyRootFilesystem must be false")
}

func TestInjectSidecar_SidecarRunAsNonRoot_True(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	require.NotNil(t, sidecar.SecurityContext)
	assert.True(t, *sidecar.SecurityContext.RunAsNonRoot)
	assert.False(t, *sidecar.SecurityContext.AllowPrivilegeEscalation)
}

func TestInjectSidecar_SPIFFEEndpointSocket_EnvVar(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("phi3", "prod")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	// Primary container must have the env var
	primary := spec.Containers[0]
	var found bool
	for _, e := range primary.Env {
		if e.Name == "SPIFFE_ENDPOINT_SOCKET" {
			found = true
			assert.Contains(t, e.Value, SPIFFEWorkloadAPIPath)
		}
	}
	assert.True(t, found, "SPIFFE_ENDPOINT_SOCKET env var must be injected into the primary container")
}

func TestInjectSidecar_SidecarContainsSPIFFEID(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("gemma", "staging")
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "vllm"}},
	}

	r.InjectSidecar(spec, svc)

	sidecar := spec.Containers[1]
	var found bool
	for _, e := range sidecar.Env {
		if e.Name == "CKODEX_SPIFFE_ID" {
			found = true
			assert.Contains(t, e.Value, "staging")
			assert.Contains(t, e.Value, "gemma")
		}
	}
	assert.True(t, found, "CKODEX_SPIFFE_ID env var must be injected into the sidecar")
}

func TestInjectSidecar_NoPrimaryContainer_NoCrash(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("phi3", "default")
	spec := &corev1.PodSpec{} // no containers

	// Must not panic
	assert.NotPanics(t, func() {
		r.InjectSidecar(spec, svc)
	})
}

// ---- NetworkPolicyReconciler.ReconcileNetworkPolicies ----------------------

func TestReconcileNetworkPolicies_DenyAllCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-deny-all-ingress", Namespace: "default"}, &policy))

	assert.Contains(t, policy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Empty(t, policy.Spec.Ingress, "deny-all must have empty ingress rules")
}

func TestReconcileNetworkPolicies_AllowGatewayCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-allow-gateway", Namespace: "default"}, &policy))

	require.Len(t, policy.Spec.Ingress, 1)
	ports := policy.Spec.Ingress[0].Ports
	require.Len(t, ports, 2)
	assert.Equal(t, int32(8000), ports[0].Port.IntVal)
	assert.Equal(t, int32(8001), ports[1].Port.IntVal)
}

// CRITICAL: Egress policy must permit DNS (53) so vLLM can resolve hostnames,
// and SPIRE Agent (8081) so pods can obtain SVIDs. Without these the model
// download and mTLS handshake both fail silently.
func TestReconcileNetworkPolicies_AllowEgressCreated_DNSAndSPIRE(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		types.NamespacedName{Name: "llama3-egress-lockdown", Namespace: "default"}, &policy))

	assert.Contains(t, policy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
	require.Len(t, policy.Spec.Egress, 2, "must have DNS rule + SPIRE Agent rule")

	// DNS rule: ports 53 UDP and 53 TCP
	dnsPorts := policy.Spec.Egress[0].Ports
	require.Len(t, dnsPorts, 2)
	assert.Equal(t, int32(53), dnsPorts[0].Port.IntVal, "first rule must be port 53")

	// SPIRE rule: port 8081, scoped to spire-agent pod selector
	spirePorts := policy.Spec.Egress[1].Ports
	require.Len(t, spirePorts, 1)
	assert.Equal(t, int32(8081), spirePorts[0].Port.IntVal, "SPIRE egress must be port 8081")
}

func TestReconcileNetworkPolicies_FourPoliciesCreated(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("mistral", "prod")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var list networkingv1.NetworkPolicyList
	require.NoError(t, np.List(context.Background(), &list))
	assert.Len(t, list.Items, 5)
}

func TestReconcileNetworkPolicies_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "default")

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))
	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var list networkingv1.NetworkPolicyList
	require.NoError(t, np.List(context.Background(), &list))
	assert.Len(t, list.Items, 5, "idempotent — no duplicate policies created")
}

func TestReconcileNetworkPolicies_SanitizesInvalidCIDR(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("invalid-cidr", "default")
	svc.Spec.ToolSurface = &servingv1alpha2.ToolSurface{
		AllowedCIDRs: []string{"999.999.999.999/99", "invalid-ip", "10.0.0.0/24"},
	}

	np := &NetworkPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	// Should not return error even with malformed CIDRs, should just skip them or fail gracefully
	require.NoError(t, np.ReconcileNetworkPolicy(context.Background(), svc))

	var policy networkingv1.NetworkPolicy
	require.NoError(t, np.Get(context.Background(),
		client.ObjectKey{Name: "invalid-cidr-allow-tools", Namespace: "default"}, &policy))

	// Should only have the valid CIDR
	require.Len(t, policy.Spec.Egress, 1)
	assert.Equal(t, "10.0.0.0/24", policy.Spec.Egress[0].To[0].IPBlock.CIDR)
}

// ---- EbpfReconciler.ReconcileEbpfPolicy ------------------------------------

func TestReconcileEbpfPolicy_CreatesSecurityPolicy(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-security-policy", Namespace: "default"}, &tp))
}

func TestReconcileEbpfPolicy_CreatesNetworkPolicy(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "llama3-network-policy", Namespace: "default"}, &tp))
}

func TestReconcileEbpfPolicy_SecurityPolicyKprobeIsSysExecve(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))

	var tp unstructured.Unstructured
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "isovalent.com",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Name: "phi3-security-policy", Namespace: "default"}, &tp))

	kprobes, _, _ := unstructured.NestedSlice(tp.Object, "spec", "kprobes")
	require.Len(t, kprobes, 1)
	kp := kprobes[0].(map[string]interface{})
	assert.Equal(t, "sys_execve", kp["call"])
}

// ---- SPIREReconciler.ReconcileSecurityPolicy (placeholder) -----------------

func TestReconcileSecurityPolicy_NoError(t *testing.T) {
	r := &SPIREReconciler{}
	svc := minimalLLMSvc("llama3", "default")
	require.NoError(t, r.ReconcileSecurityPolicy(context.Background(), svc))
}

// ---- validateSPIFFEID ------------------------------------------------------

func TestValidateSPIFFEID_Valid(t *testing.T) {
	require.NoError(t, validateSPIFFEID("default", "vllm-sa", "llama3"))
}

func TestValidateSPIFFEID_EmptyNamespace_Error(t *testing.T) {
	// An empty namespace produces path /ns//sa/vllm-sa/model/m — invalid SPIFFE path.
	err := validateSPIFFEID("", "vllm-sa", "model")
	require.Error(t, err)
}

func TestValidateSPIFFEID_ValidComplexNames(t *testing.T) {
	// Hyphens and numbers are permitted in K8s names and SPIFFE paths.
	require.NoError(t, validateSPIFFEID("prod-east", "llama3-sa", "llama-3-8b"))
}

// ---- SPIRERegistrationReconciler -------------------------------------------

func TestReconcileRegistrationEntry_CreatesConfigMap(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-llama3",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	assert.Contains(t, cm.Data["entry.json"], "spiffe://ckodex.com")
	assert.Equal(t, "true", cm.Labels["spire.ckodex.com/registration-entry"])
}

func TestReconcileRegistrationEntry_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("phi3", "staging")

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
}

func TestReconcileRegistrationEntry_EntryContainsTTL(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("gemma", "prod")

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "prod-gemma",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	// TTL of 3600 must be present in the serialised entry
	assert.Contains(t, cm.Data["entry.json"], "3600")
}

func TestReconcileRegistrationEntry_EntryContainsDNSSAN(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("mistral", "default")

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))

	var cm corev1.ConfigMap
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-mistral",
			Namespace: SPIRERegistrationNamespace,
		}, &cm))

	// DNS SAN for in-cluster DNS resolution
	assert.Contains(t, cm.Data["entry.json"], "mistral.default.svc.cluster.local")
}

func TestDeleteRegistrationEntry_RemovesConfigMap(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("llama3", "default")

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileRegistrationEntry(context.Background(), svc))
	require.NoError(t, r.DeleteRegistrationEntry(context.Background(), "default", "llama3"))

	var cm corev1.ConfigMap
	err := r.Get(context.Background(),
		types.NamespacedName{
			Name:      SPIRERegistrationCMPrefix + "default-llama3",
			Namespace: SPIRERegistrationNamespace,
		}, &cm)
	assert.True(t, err != nil, "ConfigMap should be deleted")
}

func TestDeleteRegistrationEntry_NotFound_NoError(t *testing.T) {
	scheme := secScheme(t)

	r := &SPIRERegistrationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	// Deleting a non-existent entry must be idempotent
	require.NoError(t, r.DeleteRegistrationEntry(context.Background(), "default", "nonexistent"))
}

func TestReconcileEbpfPolicy_Idempotent(t *testing.T) {
	scheme := secScheme(t)
	svc := minimalLLMSvc("gemma", "default")

	r := &EbpfReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))
	require.NoError(t, r.ReconcileEbpfPolicy(context.Background(), svc))
}
