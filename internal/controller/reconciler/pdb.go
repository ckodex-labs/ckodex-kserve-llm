package reconciler

import (
	"context"
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// PDBReconciler manages the PodDisruptionBudget for the inference deployment.
type PDBReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates or updates a PodDisruptionBudget.
func (r *PDBReconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)

	labels := map[string]string{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
	}

	minAvailable := intstr.FromInt32(1)
	desired := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Name,
			Namespace: llmSvc.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference on PDB: %w", err)
	}

	var existing policyv1.PodDisruptionBudget
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		logger.Info("creating PodDisruptionBudget", "name", desired.Name)
		return r.Client.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get PDB: %w", err)
	}

	// Update only if spec changed
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		logger.Info("updating PodDisruptionBudget", "name", desired.Name)
		return r.Client.Update(ctx, &existing)
	}

	return nil
}
