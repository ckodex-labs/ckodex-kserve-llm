package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type coverageClient struct {
	client.Client
	getErr, listErr, createErr, updateErr, deleteErr error
}

func (c *coverageClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.getErr != nil {
		return c.getErr
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *coverageClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listErr != nil {
		return c.listErr
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *coverageClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createErr != nil {
		return c.createErr
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *coverageClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *coverageClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestLLMInferenceCoverage_HFCSIReconcileCreateUpdateAndErrors(t *testing.T) {
	scheme := buildLLMScheme(t)
	svc := makeLLMInferenceService("hf", "models")
	svc.Spec.Model.URI = "hf-mount://org/model@main"
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &HFCSIReconciler{Client: base, Scheme: scheme}
	require.NoError(t, r.Reconcile(context.Background(), svc))

	var pv corev1.PersistentVolume
	require.NoError(t, base.Get(context.Background(), types.NamespacedName{Name: HFPVName(svc)}, &pv))
	var pvc corev1.PersistentVolumeClaim
	require.NoError(t, base.Get(context.Background(), types.NamespacedName{Name: HFPVName(svc), Namespace: svc.Namespace}, &pvc))
	require.NotNil(t, pvc.OwnerReferences)

	// A CSI drift updates only the mutable CSI block; an unchanged PV is a no-op.
	pv.Spec.CSI.VolumeAttributes["revision"] = "old"
	require.NoError(t, base.Update(context.Background(), &pv))
	require.NoError(t, r.Reconcile(context.Background(), svc))
	require.NoError(t, r.Reconcile(context.Background(), svc))

	badGet := &HFCSIReconciler{Client: &coverageClient{Client: base, getErr: errors.New("lookup failed")}, Scheme: scheme}
	require.ErrorContains(t, badGet.Reconcile(context.Background(), svc), "hf-csi PV: lookup failed")

	badCreate := &HFCSIReconciler{Client: &coverageClient{Client: base, createErr: errors.New("create failed")}, Scheme: scheme}
	missing := makeLLMInferenceService("hf-create-error", "models")
	missing.Spec.Model.URI = svc.Spec.Model.URI
	require.ErrorContains(t, badCreate.Reconcile(context.Background(), missing), "hf-csi PV: create failed")

	plain := makeLLMInferenceService("plain", "models")
	require.NoError(t, r.Reconcile(context.Background(), plain))
}

func TestLLMInferenceCoverage_HFCSIImmutablePVCAndParsing(t *testing.T) {
	scheme := buildLLMScheme(t)
	svc := makeLLMInferenceService("hf", "models")
	svc.Spec.Model.URI = "hf-mount://org/model"
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
	r := &HFCSIReconciler{Client: base, Scheme: scheme}
	name := HFPVName(svc)
	oldPVC := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: svc.Namespace}}
	require.NoError(t, base.Create(context.Background(), oldPVC))
	require.NoError(t, r.Reconcile(context.Background(), svc))

	require.Equal(t, "org/model", func() string { repo, _ := parseHFMountURI("hf-mount://org/model"); return repo }())
	long := makeLLMInferenceService(strings.Repeat("n", 300), "models")
	require.Len(t, HFPVName(long), 253)
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: svc.Namespace}, oldPVC)
	require.NoError(t, err)
	require.False(t, apierrors.IsNotFound(err))
}
