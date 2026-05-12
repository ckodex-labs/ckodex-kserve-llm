package cleanup

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

type Reconciler struct {
	Client client.Client
}

func (r *Reconciler) HandleFinalizer(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService, finalizer string, cleanupFunc func() error) (bool, error) {
	if llmSvc.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(llmSvc, finalizer) {
			controllerutil.AddFinalizer(llmSvc, finalizer)
			if err := r.Client.Update(ctx, llmSvc); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	// Deletion in progress
	if controllerutil.ContainsFinalizer(llmSvc, finalizer) {
		// Execute external dependency cleanup logic
		if cleanupFunc != nil {
			if err := cleanupFunc(); err != nil {
				return false, err
			}
		}

		controllerutil.RemoveFinalizer(llmSvc, finalizer)
		if err := r.Client.Update(ctx, llmSvc); err != nil {
			return false, err
		}
	}
	return true, nil
}
