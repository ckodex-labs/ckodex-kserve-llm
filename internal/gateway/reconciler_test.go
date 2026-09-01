/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// buildScheme returns a scheme with servingv1alpha2 + Gateway API types registered.
func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, gwapiv1.Install(s))
	return s
}

// ---- BuildCanaryHTTPRoute --------------------------------------------------

func TestBuildCanaryHTTPRoute_TwoBackendsPerRule(t *testing.T) {
	llmSvc := canarySvc("canary-model", "default", "stable-model", 20)
	route := BuildCanaryHTTPRoute(llmSvc, nil)

	require.Len(t, route.Spec.Rules, 7, "same 7 paths as standard route")
	for i, rule := range route.Spec.Rules {
		assert.Len(t, rule.BackendRefs, 2, "rule[%d] must have canary+stable backends", i)
	}
}

func TestBuildCanaryHTTPRoute_WeightDistribution(t *testing.T) {
	llmSvc := canarySvc("canary", "default", "stable", 30)
	route := BuildCanaryHTTPRoute(llmSvc, nil)

	for i, rule := range route.Spec.Rules {
		canary := rule.BackendRefs[0]
		stable := rule.BackendRefs[1]
		require.NotNil(t, canary.Weight, "rule[%d] canary weight must be set", i)
		require.NotNil(t, stable.Weight, "rule[%d] stable weight must be set", i)
		assert.Equal(t, int32(30), *canary.Weight, "rule[%d] canary weight", i)
		assert.Equal(t, int32(70), *stable.Weight, "rule[%d] stable weight (100-30)", i)
	}
}

func TestBuildCanaryHTTPRoute_BackendNames(t *testing.T) {
	llmSvc := canarySvc("new-model", "prod", "base-model", 10)
	route := BuildCanaryHTTPRoute(llmSvc, nil)

	for i, rule := range route.Spec.Rules {
		assert.Equal(t, gwapiv1.ObjectName("new-model"), rule.BackendRefs[0].Name, "rule[%d] canary backend name", i)
		assert.Equal(t, gwapiv1.ObjectName("base-model"), rule.BackendRefs[1].Name, "rule[%d] stable backend name", i)
	}
}

func TestBuildCanaryHTTPRoute_Labels(t *testing.T) {
	llmSvc := canarySvc("can", "ns", "base", 25)
	route := BuildCanaryHTTPRoute(llmSvc, nil)

	assert.Equal(t, "true", route.Labels["serving.ckodex.com/canary"])
	assert.Equal(t, "25", route.Labels["serving.ckodex.com/canary-weight"])
	assert.Equal(t, "base", route.Labels["serving.ckodex.com/base-model"])
}

func TestBuildCanaryHTTPRoute_Annotations(t *testing.T) {
	llmSvc := canarySvc("can", "ns", "base", 25)
	route := BuildCanaryHTTPRoute(llmSvc, nil)

	assert.Equal(t, "25", route.Annotations["serving.ckodex.com/canary-weight"])
	assert.Equal(t, "base", route.Annotations["serving.ckodex.com/base-model"])
}

func TestBuildCanaryHTTPRoute_Name(t *testing.T) {
	llmSvc := canarySvc("my-model", "default", "stable", 10)
	route := BuildCanaryHTTPRoute(llmSvc, nil)
	assert.Equal(t, "my-model-httproute", route.Name)
}

func TestBuildCanaryHTTPRoute_Hostnames(t *testing.T) {
	llmSvc := canarySvc("can", "ns", "base", 10)
	llmSvc.Spec.Router.Route.HTTPRoute = &servingv1alpha2.HTTPRouteSpec{
		Hostnames: []string{"can.example.com"},
	}
	route := BuildCanaryHTTPRoute(llmSvc, nil)
	require.Len(t, route.Spec.Hostnames, 1)
	assert.Equal(t, gwapiv1.Hostname("can.example.com"), route.Spec.Hostnames[0])
}

func TestBuildCanaryHTTPRoute_NoHostnames_Empty(t *testing.T) {
	llmSvc := canarySvc("can", "ns", "base", 10)
	route := BuildCanaryHTTPRoute(llmSvc, nil)
	assert.Empty(t, route.Spec.Hostnames)
}

// ---- Reconciler.Reconcile — managed gateway mode ---------------------------

func TestReconciler_Reconcile_ManagedGateway_CreatesResources(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme}
	err := r.Reconcile(context.Background(), llmSvc)
	require.NoError(t, err)

	// Gateway must be created.
	var gw gwapiv1.Gateway
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-gateway", Namespace: "default"}, &gw))
	assert.Equal(t, gwapiv1.ObjectName("envoy"), gw.Spec.GatewayClassName)

	// HTTPRoute must be created.
	var route gwapiv1.HTTPRoute
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-httproute", Namespace: "default"}, &route))
}

func TestReconciler_Reconcile_GRPCDisabledByDefault_NoGRPCRoute(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme, EnableGRPC: false}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var grpcRoute gwapiv1.GRPCRoute
	err := fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-grpcroute", Namespace: "default"}, &grpcRoute)
	assert.Error(t, err, "GRPCRoute must not be created when EnableGRPC=false")
}

func TestReconciler_Reconcile_EnableGRPC_CreatesGRPCRoute(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme, EnableGRPC: true}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var grpcRoute gwapiv1.GRPCRoute
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-grpcroute", Namespace: "default"}, &grpcRoute))
}

func TestReconciler_Reconcile_GRPCEnabled_Gateway_Has2Listeners(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme, EnableGRPC: true}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var gw gwapiv1.Gateway
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-gateway", Namespace: "default"}, &gw))
	assert.Len(t, gw.Spec.Listeners, 2, "HTTP listener + gRPC listener")
}

// ---- Reconciler — existing resource update ---------------------------------

func TestReconciler_Reconcile_UpdatesExistingGateway(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")

	// Pre-existing gateway with stale GatewayClassName.
	existing := &gwapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-gateway", Namespace: "default"},
		Spec: gwapiv1.GatewaySpec{
			GatewayClassName: "stale-class",
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc, existing).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var updated gwapiv1.Gateway
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-gateway", Namespace: "default"}, &updated))
	assert.Equal(t, gwapiv1.ObjectName("envoy"), updated.Spec.GatewayClassName,
		"gateway class must be updated to desired value")
}

func TestReconciler_Reconcile_UpdatesExistingHTTPRoute(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")

	existing := &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-httproute", Namespace: "default"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc, existing).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var updated gwapiv1.HTTPRoute
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-httproute", Namespace: "default"}, &updated))
	assert.Len(t, updated.Spec.Rules, 10, "updated route must carry 10 path rules (7 original + /version, /server_info, /v1/responses)")
}

func TestReconciler_Reconcile_IsIdempotent(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	require.NoError(t, r.Reconcile(context.Background(), llmSvc))
	var first gwapiv1.HTTPRoute
	require.NoError(t, fakeClient.Get(context.Background(), k8stypes.NamespacedName{
		Name: "svc-httproute", Namespace: "default",
	}, &first))

	require.NoError(t, r.Reconcile(context.Background(), llmSvc))
	var second gwapiv1.HTTPRoute
	require.NoError(t, fakeClient.Get(context.Background(), k8stypes.NamespacedName{
		Name: "svc-httproute", Namespace: "default",
	}, &second))

	assert.Equal(t, first.ResourceVersion, second.ResourceVersion,
		"unchanged desired state must not issue another HTTPRoute write")
}

// ---- Reconciler — existing mode (no managed gateway) ----------------------

func TestReconciler_Reconcile_ExistingGatewayMode_NoGatewayCreated(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	// Unset managed, use existing gateway ref.
	llmSvc.Spec.Router.Gateway.Managed = nil
	llmSvc.Spec.Router.Gateway.ExistingRef = &servingv1alpha2.GatewayRef{
		Name: "shared-gateway", Namespace: "infra",
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	// No gateway resource should be created.
	var gw gwapiv1.Gateway
	err := fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-gateway", Namespace: "default"}, &gw)
	assert.Error(t, err, "gateway must not be created in existing-ref mode")
}

// ---- Reconciler — canary route ---------------------------------------------

func TestReconciler_Reconcile_WithCanary_CreatesCanaryHTTPRoute(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("new-model", "default")
	llmSvc.Spec.Router.Gateway.Managed = nil
	llmSvc.Spec.Router.Gateway.ExistingRef = &servingv1alpha2.GatewayRef{
		Name: "shared-gw", Namespace: "infra",
	}
	llmSvc.Spec.Canary = &servingv1alpha2.CanarySpec{Weight: 15, BaseModel: "stable-model"}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var route gwapiv1.HTTPRoute
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "new-model-httproute", Namespace: "default"}, &route))

	// Canary route has 2 backends per rule.
	for i, rule := range route.Spec.Rules {
		assert.Len(t, rule.BackendRefs, 2, "rule[%d] must have 2 backends for canary", i)
	}
}

// ---- Reconciler — reconcileGRPCRoute update --------------------------------

func TestReconciler_Reconcile_UpdatesExistingGRPCRoute(t *testing.T) {
	scheme := buildScheme(t)
	llmSvc := baseLLMSvcWithUID("svc", "default")
	existing := &gwapiv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-grpcroute", Namespace: "default"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc, existing).Build()

	r := &Reconciler{Client: fakeClient, Scheme: scheme, EnableGRPC: true}
	require.NoError(t, r.Reconcile(context.Background(), llmSvc))

	var updated gwapiv1.GRPCRoute
	require.NoError(t, fakeClient.Get(context.Background(),
		k8stypes.NamespacedName{Name: "svc-grpcroute", Namespace: "default"}, &updated))
	assert.Len(t, updated.Spec.Rules, 6, "updated grpc route must carry 6 method rules")
}

// ---- helpers ----------------------------------------------------------------

func canarySvc(name, namespace, baseModel string, weight int32) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/" + name, Name: name},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{GatewayClassName: "envoy"},
				},
			},
			Canary: &servingv1alpha2.CanarySpec{Weight: weight, BaseModel: baseModel},
		},
	}
}

// baseLLMSvcWithUID creates an LLMInferenceService with a UID set so
// controllerutil.SetControllerReference works in fake-client tests.
func baseLLMSvcWithUID(name, namespace string) *servingv1alpha2.LLMInferenceService {
	svc := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       k8stypes.UID("test-uid-" + name),
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{URI: "hf://test/" + name, Name: name},
			Router: servingv1alpha2.RouterSpec{
				Gateway: servingv1alpha2.GatewaySpec{
					Managed: &servingv1alpha2.ManagedGatewaySpec{GatewayClassName: "envoy"},
				},
			},
		},
	}
	return svc
}
