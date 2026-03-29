//go:build e2e

/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package e2e contains end-to-end tests for the CKodex KServe LLM Operator.
// These tests run against a real Kubernetes cluster (e.g. KIND) and require
// the operator to be deployed and all CRDs to be installed.
//
// Prerequisites:
//
//	export E2E_KUBECONFIG=/path/to/kubeconfig
//	make e2e-test
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	// e2eNamespace is created once per E2E run and deleted on teardown.
	e2eNamespace = "ckodex-e2e-test"

	// eventuallyTimeout is the maximum time to wait for a condition.
	eventuallyTimeout = 5 * time.Minute

	// eventuallyPoll is the polling interval for condition checks.
	eventuallyPoll = 2 * time.Second

	// shortTimeout is used for quick existence checks (finalizer, object created).
	shortTimeout = 30 * time.Second
)

// package-level state set by TestMain, used by all test functions.
var (
	e2eClient rest.Config
	k8sClient client.Client
	e2eScheme *k8sruntime.Scheme
)

// TestMain bootstraps the E2E environment: loads kubeconfig, installs CRDs,
// creates the test namespace, runs all tests, and tears down.
func TestMain(m *testing.M) {
	kubeconfig := os.Getenv("E2E_KUBECONFIG")
	if kubeconfig == "" {
		fmt.Println("E2E_KUBECONFIG not set — skipping")
		os.Exit(0)
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load kubeconfig %s: %v\n", kubeconfig, err)
		os.Exit(1)
	}
	e2eClient = *cfg

	// Build scheme: core + apps + our CRDs + gateway-api.
	scheme := k8sruntime.NewScheme()
	mustScheme(clientgoscheme.AddToScheme(scheme))
	mustScheme(appsv1.AddToScheme(scheme))
	mustScheme(apiextensionsv1.AddToScheme(scheme))
	mustScheme(servingv1alpha2.AddToScheme(scheme))
	mustScheme(gatewayv1.Install(scheme))
	e2eScheme = scheme

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build client: %v\n", err)
		os.Exit(1)
	}
	k8sClient = cl

	ctx := context.Background()

	// Apply all CRDs from config/crd/ using server-side apply.
	if err := applyCRDs(ctx, cl); err != nil {
		fmt.Fprintf(os.Stderr, "apply CRDs: %v\n", err)
		os.Exit(1)
	}

	// Create test namespace (idempotent).
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: e2eNamespace}}
	if err := cl.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		fmt.Fprintf(os.Stderr, "create namespace %s: %v\n", e2eNamespace, err)
		os.Exit(1)
	}

	code := m.Run()

	// Teardown: delete test namespace (best-effort).
	_ = cl.Delete(ctx, ns)

	os.Exit(code)
}

// waitForCondition polls obj (by its key) until condFn returns true, an error,
// or timeout is reached. Uses wait.PollUntilContextTimeout.
func waitForCondition(
	t *testing.T,
	key client.ObjectKey,
	obj client.Object,
	timeout time.Duration,
	condFn func(client.Object) (bool, error),
) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return wait.PollUntilContextTimeout(ctx, eventuallyPoll, timeout, true,
		func(_ context.Context) (bool, error) {
			fresh := obj.DeepCopyObject().(client.Object)
			if err := k8sClient.Get(ctx, key, fresh); err != nil {
				if errors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return condFn(fresh)
		},
	)
}

// waitForAbsence polls until obj is gone (GET returns NotFound).
func waitForAbsence(t *testing.T, key client.ObjectKey, obj client.Object, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return wait.PollUntilContextTimeout(ctx, eventuallyPoll, timeout, true,
		func(_ context.Context) (bool, error) {
			fresh := obj.DeepCopyObject().(client.Object)
			err := k8sClient.Get(ctx, key, fresh)
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		},
	)
}

// repoRoot returns the repository root, two levels above this file.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// applyCRDs reads every YAML file under config/crd/ and applies them via
// server-side apply using the unstructured dynamic path on client.Client.
func applyCRDs(ctx context.Context, cl client.Client) error {
	crdDir := filepath.Join(repoRoot(), "config", "crd")
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		return fmt.Errorf("read crd dir %s: %w", crdDir, err)
	}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".yaml" && filepath.Ext(name) != ".yml" {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(crdDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		obj := &unstructured.Unstructured{}
		if _, _, err := dec.Decode(raw, nil, obj); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}

		if err := cl.Patch(ctx, obj, client.Apply,
			client.ForceOwnership,
			client.FieldOwner("ckodex-e2e"),
		); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// mustScheme panics if scheme registration fails — programming error only.
func mustScheme(err error) {
	if err != nil {
		panic("scheme registration: " + err.Error())
	}
}
