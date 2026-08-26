package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

func TestSkillRegistryReconcile_ValidRegistry_ReadyTrue(t *testing.T) {
	scheme := newControllerScheme(t)
	reg := skillReg("my-registry", validEntry("skill-a"), validEntry("skill-b"))
	r := &SkillRegistryReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(reg).WithStatusSubresource(reg).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "my-registry", Namespace: "default"}})
	require.NoError(t, err)
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "my-registry", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	updated := &servingv1alpha2.SkillRegistry{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "my-registry", Namespace: "default"}, updated))
	assert.Contains(t, updated.Finalizers, api.FinalizerName)
	assert.Equal(t, int32(2), updated.Status.EntryCount)
}

func TestSkillRegistryReconcile_InvalidEntry_ReadyFalse(t *testing.T) {
	scheme := newControllerScheme(t)
	reg := skillReg("bad-registry", servingv1alpha2.SkillEntry{Name: "bad", Endpoint: "http://bad:8080"})
	r := &SkillRegistryReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(reg).WithStatusSubresource(reg).Build(), Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad-registry", Namespace: "default"}})
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad-registry", Namespace: "default"}})
	require.NoError(t, err)
	updated := &servingv1alpha2.SkillRegistry{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "bad-registry", Namespace: "default"}, updated))
	assert.Equal(t, int32(0), updated.Status.EntryCount)
}

func TestSkillRegistryReconcile_NotFound_NoError(t *testing.T) {
	scheme := newControllerScheme(t)
	r := &SkillRegistryReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestBoolToConditionStatus(t *testing.T) {
	assert.Equal(t, metav1.ConditionTrue, boolToConditionStatus(true))
	assert.Equal(t, metav1.ConditionFalse, boolToConditionStatus(false))
}
