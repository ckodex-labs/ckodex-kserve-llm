/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package integration contains envtest-based integration tests for all CKodex
// operator controllers. Tests run against a real API server + etcd binary
// provisioned by controller-runtime's envtest framework.
//
// Prerequisites:
//
//	export KUBEBUILDER_ASSETS=$(go env GOPATH)/pkg/mod/sigs.k8s.io/controller-runtime@$(go list -m -f '{{.Version}}' sigs.k8s.io/controller-runtime)/pkg/envtest/assets
//	Or install kubebuilder-tools: https://github.com/kubernetes-sigs/controller-runtime/releases
package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/autoscaler"
	"github.com/ckodex-labs/kserve-llm-operator/internal/chaos"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller"
	"github.com/ckodex-labs/kserve-llm-operator/internal/gateway"
)

const (
	// testNamespace is created once per test suite run.
	testNamespace = "ckodex-integration-test"

	// eventuallyTimeout is the poll deadline for Eventually assertions.
	eventuallyTimeout = 60 * time.Second

	// eventuallyInterval is the polling interval.
	eventuallyInterval = 250 * time.Millisecond
)

// suiteEnv holds the shared envtest environment for all integration tests.
// Individual test files access it through the package-level vars below.
type suiteEnv struct {
	cfg    *rest.Config
	client client.Client
	env    *envtest.Environment
	ctx    context.Context
	cancel context.CancelFunc
	chaos  *chaos.Engine
}

// suite is the package-level test environment, initialised in TestMain.
var suite suiteEnv

// TestMain bootstraps the envtest environment and runs all integration tests.
// Using TestMain (rather than TestXxx) gives us a single setup/teardown cycle
// for all tests in the package — critical for envtest which is slow to start.
func TestMain(m *testing.M) {
	// Initialize logger for envtest diagnostics
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	// Skip if running without kubebuilder assets (e.g., CI without the binary set).
	// Developers must set KUBEBUILDER_ASSETS to run these tests locally.
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Attempt the default location used by controller-runtime's Makefile
		defaultAssets := filepath.Join(repoRoot(), "bin", "k8s")
		if _, err := os.Stat(defaultAssets); os.IsNotExist(err) {
			// FACT: envtest binaries absent — skip gracefully rather than fail.
			// Operators can still gate on this in CI by setting KUBEBUILDER_ASSETS.
			os.Exit(0)
		}
	}

	// Build the scheme with all required types.
	scheme := k8sruntime.NewScheme()
	must(clientgoscheme.AddToScheme(scheme))
	must(servingv1alpha2.AddToScheme(scheme))
	must(gwapiv1.Install(scheme))

	// Locate CRD manifests relative to the repo root.
	crdPath := filepath.Join(repoRoot(), "config", "crd")

	// Locate Gateway API CRDs dynamically to ensure they are available to envtest.
	gwCrdPath := ""
	out, errCmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/gateway-api").Output()
	if errCmd == nil {
		gwCrdPath = filepath.Join(strings.TrimSpace(string(out)), "config", "crd", "standard")
	}

	crdPaths := []string{crdPath}
	if gwCrdPath != "" {
		crdPaths = append(crdPaths, gwCrdPath)
	}

	suite.env = &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme,
	}

	// Bump etcd max request sizes via its native environment variable to allow
	// our large CRDs (resulting from allowDangerousTypes=true floats) to be
	// installed without the "request is too large" panic.
	_ = os.Setenv("ETCD_MAX_REQUEST_BYTES", "52428800")

	var mgr ctrl.Manager
	var err error
	suite.cfg, err = suite.env.Start()
	mustf(err, "start envtest")

	suite.client, err = client.New(suite.cfg, client.Options{Scheme: scheme})
	mustf(err, "build client")

	suite.ctx, suite.cancel = context.WithCancel(context.Background())
	suite.chaos = chaos.NewEngine(suite.client)

	// Start the controller manager so reconcilers run during tests.
	// We disable metrics and health probes to avoid port conflicts on the host.
	mgr, err = ctrl.NewManager(suite.cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	})
	mustf(err, "new manager")

	mustf((&controller.AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr), "setup agent controller")

	mustf((&controller.SkillRegistryReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr), "setup skillregistry controller")

	mustf((&controller.ModelOnboardingReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr), "setup modelonboarding controller")

	mustf((&controller.LLMInferenceServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Gateway: &gateway.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
		Autoscaler: &autoscaler.Reconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
		EnableGRPC: true,
	}).SetupWithManager(mgr), "setup llm controller")

	mustf((&controller.MultimodalInferenceServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr), "setup multimodal controller")

	go func() {
		mustf(mgr.Start(suite.ctx), "manager start")
	}()

	// Wait for caches to sync before running tests to avoid race conditions.
	// This ensures that the manager is fully operational.
	if !mgr.GetCache().WaitForCacheSync(suite.ctx) {
		mustf(fmt.Errorf("cache sync failed"), "waiting for caches")
	}

	// Create the shared test namespace.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	mustf(suite.client.Create(suite.ctx, ns), "create test namespace")

	code := m.Run()

	suite.cancel()
	mustf(suite.env.Stop(), "stop envtest")
	os.Exit(code)
}

// uniqueID returns a monotonically increasing integer suitable for constructing
// unique resource names within a test run. Using a counter avoids name collisions
// when parallel tests create resources in the same namespace.
var uniqueIDCounter int64

func uniqueID() int64 {
	return atomic.AddInt64(&uniqueIDCounter, 1)
}

// repoRoot returns the path to the repository root by walking up from the test
// file's location.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// --- helpers ---

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustf(err error, msg string) {
	if err != nil {
		panic(msg + ": " + err.Error())
	}
}

// newLLMInferenceService returns a minimal ready-state LLMInferenceService
// suitable for use as a referenced model in other tests.
func newLLMInferenceService(t *testing.T, name string) *servingv1alpha2.LLMInferenceService {
	t.Helper()
	llm := &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "model-server",
						Image: "ckodex/model-server:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, suite.client.Create(suite.ctx, llm))

	// Create a dummy deployment so the LLMInferenceService controller can detect it
	// and set the Ready status condition naturally.
	labels := map[string]string{
		"app.kubernetes.io/name":     "llminferenceservice",
		"app.kubernetes.io/instance": name,
	}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: servingv1alpha2.SchemeGroupVersion.String(),
					Kind:       "LLMInferenceService",
					Name:       llm.Name,
					UID:        llm.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: llm.Spec.Template,
		},
	}
	if dep.Spec.Template.Labels == nil {
		dep.Spec.Template.Labels = make(map[string]string)
	}
	for k, v := range labels {
		dep.Spec.Template.Labels[k] = v
	}
	require.NoError(t, suite.client.Create(suite.ctx, dep))

	// Mock Deployment status to satisfy the controller
	patchDep := client.MergeFrom(dep.DeepCopy())
	dep.Status.Replicas = 1
	dep.Status.ReadyReplicas = 1
	require.NoError(t, suite.client.Status().Patch(suite.ctx, dep, patchDep))

	// Wait for the LLMInferenceService to become Ready naturally
	err := wait.PollUntilContextTimeout(suite.ctx, eventuallyInterval, eventuallyTimeout, true,
		func(ctx context.Context) (bool, error) {
			var s servingv1alpha2.LLMInferenceService
			if err := suite.client.Get(ctx, client.ObjectKeyFromObject(llm), &s); err != nil {
				return false, nil
			}
			return s.Status.ModelReady && s.Status.Replicas > 0, nil
		},
	)
	require.NoError(t, err, "LLMInferenceService should become ready after mocking deployment")

	t.Cleanup(func() {
		_ = suite.client.Delete(context.Background(), llm)
	})
	return llm
}

// mockPodReady manually updates a Pod's status to Ready in envtest.
// This is required because envtest does not run a kubelet to perform these updates.
func mockPodReady(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	patch := client.MergeFrom(pod.DeepCopy())
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:               corev1.PodReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
		},
	}
	require.NoError(t, suite.client.Status().Patch(suite.ctx, pod, patch))
}
