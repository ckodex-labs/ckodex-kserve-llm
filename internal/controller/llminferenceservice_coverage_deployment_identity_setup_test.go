package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/security"
)

func TestLLMInferenceCoverage_DeploymentCreateUpdateAndPrefillCleanup(t *testing.T) {
	s := buildLLMScheme(t)
	svc := makeLLMInferenceService("deploy", "default")
	base := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()
	r := setupReconciler(base, s)

	// Prefill is opt-in. Without it, an existing prefill deployment is removed.
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-prefill", Namespace: "default"}}
	require.NoError(t, base.Create(context.Background(), old))
	require.NoError(t, r.reconcilePrefillDeployment(context.Background(), svc))
	var removed appsv1.Deployment
	require.Error(t, base.Get(context.Background(), types.NamespacedName{Name: "deploy-prefill", Namespace: "default"}, &removed))

	svc.Spec.Prefill = &servingv1alpha2.PrefillSpec{}
	// A configured prefill deployment exercises the update/no-op branch after creation.
	require.NoError(t, r.reconcilePrefillDeployment(context.Background(), svc))
	require.NoError(t, r.reconcilePrefillDeployment(context.Background(), svc))

	// Existing primary deployment with no desired drift is a no-op.
	desired := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy", Namespace: "default"}}
	require.NoError(t, base.Create(context.Background(), desired))
	require.NoError(t, r.applyDeployment(context.Background(), svc, desired, 1))

	r.Client = &coverageClient{Client: base, getErr: errors.New("deployment lookup failed")}
	require.ErrorContains(t, r.applyDeployment(context.Background(), svc, desired, 1), "get deployment")

	r.Client = &coverageClient{Client: base, createErr: errors.New("deployment create failed")}
	missing := desired.DeepCopy()
	missing.Name = "missing-deployment"
	require.ErrorContains(t, r.applyDeployment(context.Background(), svc, missing, 1), "deployment create failed")
}

func TestLLMInferenceCoverage_IdentityOptionalComponents(t *testing.T) {
	s := buildLLMScheme(t)
	r := setupReconciler(fake.NewClientBuilder().WithScheme(s).Build(), s)
	state := newLLMInferenceReconcileState(makeLLMInferenceService("identity", "default"))
	require.NoError(t, r.reconcileIdentityAndPolicy(context.Background(), state))
	require.NoError(t, r.reconcileVault(context.Background(), state))
	require.NoError(t, r.reconcileSPIRE(context.Background(), state))
	require.NoError(t, r.reconcileEbpf(context.Background(), state))
	require.NoError(t, r.reconcileOPA(context.Background(), state))

	// A configured OPA reconciler with no discovery client safely skips absent
	// Gatekeeper APIs, preserving the fail-closed optional-component contract.
	r.OPA = &security.OPAReconciler{}
	require.NoError(t, r.reconcileOPA(context.Background(), state))
}

func TestLLMInferenceCoverage_SetupInitializesRuntimeComponents(t *testing.T) {
	s := buildLLMScheme(t)
	r := setupReconciler(fake.NewClientBuilder().WithScheme(s).Build(), s)
	r.RuntimeImage = "runtime:test"
	r.HFInitializerImage = "initializer:test"
	r.KServeMultiNodeRuntime = "kserve-multinode"
	mgr := &coverageManager{Client: r.Client, Scheme: s, Recorder: record.NewFakeRecorder(20)}
	r.initializeDeploymentComponents(mgr)
	r.initializeServiceComponents(mgr)
	r.initializeRuntimeComponents(mgr)
	require.NotNil(t, r.DeploymentBuilder)
	require.NotNil(t, r.ServiceReconciler)
	require.NotNil(t, r.StatusReconciler)
	require.NotNil(t, r.CleanupReconciler)
	require.NotNil(t, r.HFCSI)
	require.NotNil(t, r.KServeMultiNode)
}

type coverageManager struct {
	ctrl.Manager
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (m *coverageManager) GetClient() client.Client                        { return m.Client }
func (m *coverageManager) GetAPIReader() client.Reader                     { return m.Client }
func (m *coverageManager) GetScheme() *runtime.Scheme                      { return m.Scheme }
func (m *coverageManager) GetEventRecorderFor(string) record.EventRecorder { return m.Recorder }
