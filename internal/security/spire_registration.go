/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package security

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/spiffe/go-spiffe/v2/spiffeid"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

const (
	// SPIRERegistrationNamespace is the namespace where SPIRE is deployed.
	// Registration entry ConfigMaps are created here so the SPIRE k8s Workload
	// Attestor can discover and import them into the SPIRE server.
	SPIRERegistrationNamespace = "spire"

	// SPIRERegistrationCMPrefix is the ConfigMap name prefix for registration entries.
	SPIRERegistrationCMPrefix = "ckodex-spire-entry-"

	// DefaultSVIDTTLSeconds is the SVID time-to-live in seconds.
	// 3600s = 1 hour; SPIRE will rotate before expiry.
	DefaultSVIDTTLSeconds = int32(3600)
)

// RegistrationEntry describes a SPIRE registration entry for a workload.
// Serialised as JSON into a ConfigMap that the SPIRE k8s Workload Attestor reads.
type RegistrationEntry struct {
	SPIFFEID  string   `json:"spiffeId"`
	ParentID  string   `json:"parentId,omitempty"`
	Selectors []string `json:"selectors"`
	TTL       int32    `json:"ttl,omitempty"`
	DNSSANs   []string `json:"dnsSans,omitempty"`
}

// SPIRERegistrationReconciler manages SPIRE registration entries for LLMInferenceService workloads.
//
// SPIFFE IDs are validated with go-spiffe/v2/spiffeid before writing.
type SPIRERegistrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// SpireReconciler provides SPIFFE ID generation.
	SpireReconciler *SPIREReconciler
	// VClusterMode indicates we are running in a virtual cluster.
	VClusterMode bool
	// HostNamespace is the physical shadow namespace in the host cluster.
	HostNamespace string
}

// ReconcileRegistrationEntry creates or updates the SPIRE registration entry ConfigMap
// for the given LLMInferenceService. The service account name is inferred from the
// service name (operator convention: SA name == service name).
func (r *SPIRERegistrationReconciler) ReconcileRegistrationEntry(
	ctx context.Context,
	llmSvc *servingv1alpha2.LLMInferenceService,
) error {
	logger := log.FromContext(ctx).WithValues("component", "spire-registration",
		"service", llmSvc.Name, "namespace", llmSvc.Namespace)

	// Service account name follows operator convention: same as service name.
	sa := llmSvc.Name
	spiffeIDStr := r.SpireReconciler.SPIFFEIDForService(llmSvc.Namespace, sa, llmSvc.Name)

	// In vcluster mode, the physical namespace in the host cluster is what
	// the SPIRE Agent (running on host nodes) will see during pod attestation.
	physicalNS := llmSvc.Namespace
	if r.VClusterMode && r.HostNamespace != "" {
		physicalNS = r.HostNamespace
	}

	// Validate the SPIFFE ID components with go-spiffe/v2 before writing to cluster state.
	if err := validateSPIFFEID(llmSvc.Namespace, sa, llmSvc.Name); err != nil {
		return fmt.Errorf("SPIFFE ID validation failed for %s/%s: %w",
			llmSvc.Namespace, llmSvc.Name, err)
	}

	entry := RegistrationEntry{
		SPIFFEID: spiffeIDStr,
		Selectors: []string{
			fmt.Sprintf("k8s:ns:%s", physicalNS),
			fmt.Sprintf("k8s:sa:%s", sa),
		},
		TTL: DefaultSVIDTTLSeconds,
		DNSSANs: []string{
			fmt.Sprintf("%s.%s.svc.cluster.local", llmSvc.Name, llmSvc.Namespace),
		},
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal registration entry for %s: %w", llmSvc.Name, err)
	}

	tenantID := llmSvc.Labels["ckodex.com/tenant-id"]
	cmName := SPIRERegistrationCMPrefix + llmSvc.Namespace + "-" + llmSvc.Name

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: SPIRERegistrationNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":        "ckodex-kserve-llm-operator",
				"spire.ckodex.com/registration-entry": "true",
				"ckodex.com/tenant-id":                tenantID,
			},
			Annotations: map[string]string{
				"ckodex.com/source-namespace": llmSvc.Namespace,
				"ckodex.com/source-service":   llmSvc.Name,
				"ckodex.com/spiffe-id":        spiffeIDStr,
			},
		},
		Data: map[string]string{
			"entry.json": string(entryJSON),
		},
	}

	var existing corev1.ConfigMap
	err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: SPIRERegistrationNamespace}, &existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get spire registration configmap %s: %w", cmName, err)
		}
		if createErr := r.Create(ctx, desired); createErr != nil {
			return fmt.Errorf("create spire registration configmap %s: %w", cmName, createErr)
		}
		logger.Info("SPIRE registration entry created", "spiffeId", spiffeIDStr)
		return nil
	}

	// Update only when the entry content changed to avoid noisy reconciles.
	if existing.Data["entry.json"] != string(entryJSON) {
		existing.Data = desired.Data
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		if updateErr := r.Update(ctx, &existing); updateErr != nil {
			return fmt.Errorf("update spire registration configmap %s: %w", cmName, updateErr)
		}
		logger.Info("SPIRE registration entry updated", "spiffeId", spiffeIDStr)
	}

	return nil
}

// DeleteRegistrationEntry removes the SPIRE registration entry ConfigMap for a service.
// Called during LLMInferenceService finalizer cleanup.
func (r *SPIRERegistrationReconciler) DeleteRegistrationEntry(
	ctx context.Context,
	namespace, name string,
) error {
	cmName := SPIRERegistrationCMPrefix + namespace + "-" + name
	var cm corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: SPIRERegistrationNamespace}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already gone — idempotent.
		}
		return fmt.Errorf("get spire registration configmap %s for deletion: %w", cmName, err)
	}
	return r.Delete(ctx, &cm)
}

// validateSPIFFEID uses go-spiffe/v2/spiffeid to verify that the generated ID is
// well-formed before writing it to cluster state.
//
// go-spiffe/v2 v2.6.0 exposes IDFromPath(td, path) — we reconstruct the ID from
// components to validate both the trust domain and path simultaneously.
// This catches invalid characters, empty paths, and trust-domain mismatches.
func validateSPIFFEID(namespace, sa, modelName string) error {
	td, err := spiffeid.TrustDomainFromString(SPIFFETrustDomain)
	if err != nil {
		return fmt.Errorf("invalid trust domain %q: %w", SPIFFETrustDomain, err)
	}

	// FromPath validates the path segment (no forbidden characters, non-empty, etc.)
	path := fmt.Sprintf("/ns/%s/sa/%s/model/%s", namespace, sa, modelName)
	if _, err := spiffeid.FromPath(td, path); err != nil {
		return fmt.Errorf("invalid SPIFFE ID path %q for trust domain %q: %w",
			path, SPIFFETrustDomain, err)
	}
	return nil
}
