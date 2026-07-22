/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/controller/api"
)

// HFCSIReconciler provisions PersistentVolume + PersistentVolumeClaim pairs for
// LLMInferenceService resources that use the hf-mount:// URI scheme.
//
// It follows the official hf-csi-driver static provisioning pattern:
//   - PV carries sourceType=repo + sourceId (official attribute names — NOT "repo")
//   - Auth via nodePublishSecretRef on the PV spec (NOT inline volumeAttribute "tokenSecret")
//   - PVC binds to PV via VolumeName; carries LLMInferenceService owner ref for GC
//   - ReclaimPolicy=Delete cascades PV deletion when PVC is removed
//
// The HFCSIReconciler must be called before the deployment builder reads the pod spec,
// so the PVC exists by the time the pod is scheduled.
type HFCSIReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is a no-op for non hf-mount:// URIs.
func (r *HFCSIReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if !strings.HasPrefix(llmSvc.Spec.Model.URI, "hf-mount://") {
		return nil
	}
	logger := log.FromContext(ctx).WithValues("component", "hfcsi")
	repo, revision := parseHFMountURI(llmSvc.Spec.Model.URI)
	pvName := HFPVName(llmSvc)

	if err := r.reconcilePV(ctx, logger, llmSvc, pvName, repo, revision); err != nil {
		return fmt.Errorf("hf-csi PV: %w", err)
	}
	return r.reconcilePVC(ctx, logger, llmSvc, pvName)
}

type contextLogger interface {
	Info(string, ...any)
}

func (r *HFCSIReconciler) reconcilePV(ctx context.Context, logger contextLogger, llmSvc *servingv1alpha2.LLMInferenceService, pvName, repo, revision string) error {
	desired := r.buildPV(llmSvc, pvName, repo, revision)

	var existing corev1.PersistentVolume
	err := r.Get(ctx, types.NamespacedName{Name: pvName}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating hf-csi PV", "name", pvName)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	// PV spec is largely immutable once bound; only update the CSI block on drift.
	if !equality.Semantic.DeepEqual(existing.Spec.CSI, desired.Spec.CSI) {
		existing.Spec.CSI = desired.Spec.CSI
		return r.Update(ctx, &existing)
	}
	return nil
}

func (r *HFCSIReconciler) reconcilePVC(ctx context.Context, logger contextLogger, llmSvc *servingv1alpha2.LLMInferenceService, pvName string) error {
	desired := r.buildPVC(llmSvc, pvName)
	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("owner ref on hf-csi PVC: %w", err)
	}

	var existing corev1.PersistentVolumeClaim
	err := r.Get(ctx, types.NamespacedName{Name: pvName, Namespace: llmSvc.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating hf-csi PVC", "name", pvName)
		return r.Create(ctx, desired)
	}
	// PVC spec is immutable once bound; idempotent return is correct here.
	return err
}

func (r *HFCSIReconciler) buildPV(llmSvc *servingv1alpha2.LLMInferenceService, pvName, repo, revision string) *corev1.PersistentVolume {
	attrs := map[string]string{
		"sourceType": "repo",
		"sourceId":   repo,
	}
	if revision != "" {
		attrs["revision"] = revision
	}

	csi := &corev1.CSIPersistentVolumeSource{
		Driver:           api.HFMountCSIDriver,
		VolumeHandle:     pvName,
		VolumeAttributes: attrs,
		ReadOnly:         true,
	}
	if llmSvc.Spec.Model.Storage != nil && llmSvc.Spec.Model.Storage.SecretRef != nil {
		// The operator uses HF_TOKEN consistently for hf:// downloads. Tell the
		// CSI driver to read that same key instead of its default "token" key so
		// users do not need two differently shaped credentials.
		attrs["tokenKey"] = "HF_TOKEN"
		csi.NodePublishSecretRef = &corev1.SecretReference{
			Name:      llmSvc.Spec.Model.Storage.SecretRef.Name,
			Namespace: llmSvc.Namespace,
		}
	}

	emptyClass := ""
	cap := resource.MustParse(api.HFCSIPVStorageSize)
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-operator",
				"serving.ckodex.com/svc":       llmSvc.Name,
				"serving.ckodex.com/namespace": llmSvc.Namespace,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: cap},
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              emptyClass,
			// ClaimRef pre-binds the PV to its PVC so the dynamic provisioner cannot steal it.
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: llmSvc.Namespace,
				Name:      pvName,
			},
		},
	}
	// PersistentVolumeSource is embedded in PersistentVolumeSpec; set after construction.
	pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{CSI: csi}
	return pv
}

func (r *HFCSIReconciler) buildPVC(llmSvc *servingv1alpha2.LLMInferenceService, pvName string) *corev1.PersistentVolumeClaim {
	emptyClass := ""
	cap := resource.MustParse(api.HFCSIPVStorageSize)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvName,
			Namespace: llmSvc.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: cap}},
			StorageClassName: &emptyClass,
			// Static binding: reference the PV by name so no StorageClass provisioner is involved.
			VolumeName: pvName,
			VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem),
		},
	}
}

// parseHFMountURI returns (repo, revision) from "hf-mount://org/repo@revision".
func parseHFMountURI(uri string) (repo, revision string) {
	path := strings.TrimPrefix(uri, "hf-mount://")
	if idx := strings.Index(path, "@"); idx != -1 {
		return path[:idx], path[idx+1:]
	}
	return path, ""
}

// HFPVName returns a deterministic, Kubernetes-legal PV/PVC name for a given LLMInferenceService.
// Must match the inline computation in deployment/builder.go (no shared import allowed across packages).
func HFPVName(llmSvc *servingv1alpha2.LLMInferenceService) string {
	name := fmt.Sprintf("hf-model-%s-%s", llmSvc.Namespace, llmSvc.Name)
	if len(name) > 253 {
		return name[:253]
	}
	return name
}
