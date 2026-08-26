/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func eppScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, servingv1alpha2.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, rbacv1.AddToScheme(s))
	return s
}

func eppSvc(name, ns string) *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: servingv1alpha2.LLMInferenceServiceSpec{
			Model: servingv1alpha2.ModelSpec{
				Name: name,
				URI:  "hf://meta-llama/Llama-3.2-1B",
			},
			Router: servingv1alpha2.RouterSpec{Scheduler: &servingv1alpha2.SchedulerSpec{}},
		},
	}
}

func preprovisionedEPPServiceAccount(namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      EPPServiceAccountName,
			Namespace: namespace,
			Labels:    map[string]string{EPPServiceAccountLabel: "preprovisioned"},
		},
	}
}

// ---- parseQuantity ---------------------------------------------------------

func TestParseQuantity_CPU(t *testing.T) {
	q := parseQuantity("100m")
	require.NotNil(t, q)
	assert.Equal(t, "100m", q.String())
}

func TestParseQuantity_Memory(t *testing.T) {
	q := parseQuantity("256Mi")
	require.NotNil(t, q)
	assert.Equal(t, "256Mi", q.String())
}

// ---- eppLabels -------------------------------------------------------------

func TestEppLabels_ContainsExpectedKeys(t *testing.T) {
	svc := eppSvc("phi3", "default")
	labels := eppLabels(svc)

	assert.Equal(t, "epp", labels["app.kubernetes.io/name"])
	assert.Equal(t, "phi3-epp", labels["app.kubernetes.io/instance"])
	assert.Equal(t, "ckodex-kserve-llm-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "scheduler", labels["serving.ckodex.com/role"])
}

// ---- reconcileDeployment ---------------------------------------------------

func TestReconcileDeployment_CreatesDeployment(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("llama3", "default")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.reconcileDeployment(context.Background(), svc, 1))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "llama3-epp", Namespace: "default"}, &dep))

	assert.Equal(t, int32(1), *dep.Spec.Replicas)
	assert.Equal(t, EPPImage, dep.Spec.Template.Spec.Containers[0].Image)
}

func TestReconcileDeployment_UpdatesReplicas(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("llama3", "default")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	// Create with 1 replica
	require.NoError(t, m.reconcileDeployment(context.Background(), svc, 1))

	// Update to 3 replicas
	require.NoError(t, m.reconcileDeployment(context.Background(), svc, 3))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "llama3-epp", Namespace: "default"}, &dep))
	assert.Equal(t, int32(3), *dep.Spec.Replicas)
}

func TestReconcileDeployment_ContainsPoolArgs(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("mistral", "prod")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.reconcileDeployment(context.Background(), svc, 1))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "mistral-epp", Namespace: "prod"}, &dep))

	args := dep.Spec.Template.Spec.Containers[0].Args
	assert.Contains(t, args, "--pool-name=mistral")
	assert.Contains(t, args, "--pool-namespace=prod")
	assert.Contains(t, args, "--pool-group=inference.networking.k8s.io")
	assert.Contains(t, args, "--config-file=/config/scheduler.yaml")
	assert.Contains(t, args, "--grpc-health-port=9003")
	assert.Contains(t, args, "--secure-serving=false")
	assert.Equal(t, EPPHealthPort, dep.Spec.Template.Spec.Containers[0].ReadinessProbe.GRPC.Port)
	require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
	assert.Equal(t, "mistral-scheduler-config", dep.Spec.Template.Spec.Volumes[0].ConfigMap.Name)
}

func TestReconcileDeployment_SecurityContextSet(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("phi3", "default")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.reconcileDeployment(context.Background(), svc, 1))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "phi3-epp", Namespace: "default"}, &dep))

	sc := dep.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.RunAsNonRoot)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
}

// ---- reconcileService ------------------------------------------------------

func TestReconcileService_CreatesService(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("llama3", "default")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.reconcileService(context.Background(), svc))

	var service corev1.Service
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "llama3-epp", Namespace: "default"}, &service))

	assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
	require.Len(t, service.Spec.Ports, 1)
	assert.Equal(t, EPPPort, service.Spec.Ports[0].Port)
	assert.Equal(t, "grpc", service.Spec.Ports[0].Name)
}

func TestReconcileService_UpdatesExistingService(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("gemma", "staging")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.reconcileService(context.Background(), svc))
	require.NoError(t, m.reconcileService(context.Background(), svc))

	var service corev1.Service
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "gemma-epp", Namespace: "staging"}, &service))
	assert.Equal(t, EPPPort, service.Spec.Ports[0].Port)
}

// ---- Reconcile (full dispatch) ---------------------------------------------

func TestEPPManager_Reconcile_DefaultReplicas(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("llama3", "default")
	// No Replicas set → defaults to 1

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, preprovisionedEPPServiceAccount("default")).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.Reconcile(context.Background(), svc))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "llama3-epp", Namespace: "default"}, &dep))
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
	assert.Equal(t, EPPServiceAccountName, dep.Spec.Template.Spec.ServiceAccountName)
}

func TestEPPManager_Reconcile_CustomReplicas(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("phi3", "production")
	svc.Spec.Router.Scheduler.Replicas = ptr.To(int32(3))

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, preprovisionedEPPServiceAccount("production")).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.Reconcile(context.Background(), svc))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "phi3-epp", Namespace: "production"}, &dep))
	assert.Equal(t, int32(3), *dep.Spec.Replicas)
}

func TestEPPManager_Reconcile_CreatesServiceAndDeployment(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("mistral", "default")

	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, preprovisionedEPPServiceAccount("default")).Build(),
		Scheme: scheme,
	}

	require.NoError(t, m.Reconcile(context.Background(), svc))

	var dep appsv1.Deployment
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "mistral-epp", Namespace: "default"}, &dep))

	var service corev1.Service
	require.NoError(t, m.Get(context.Background(),
		types.NamespacedName{Name: "mistral-epp", Namespace: "default"}, &service))
}

func TestEPPManager_Reconcile_RequiresPreProvisionedRBAC(t *testing.T) {
	scheme := eppScheme(t)
	svc := eppSvc("mistral", "default")
	m := &EPPManager{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Scheme: scheme,
	}

	err := m.Reconcile(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-provisioned")
}
