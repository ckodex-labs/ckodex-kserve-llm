/*
Copyright 2026 CKodex Authors.
*/

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecurityReconcilers_ResilienceToMissingCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	// Do NOT register any CRDs in this scheme to simulate missing CRDs

	ctx := context.Background()

	t.Run("ToolSurfaceReconciler", func(t *testing.T) {
		r := &ToolSurfaceReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme: scheme,
		}
		svc := minimalLLMSvc("test-svc", "default")
		err := r.ReconcileToolSurface(ctx, svc, nil)
		assert.NoError(t, err, "ToolSurfaceReconciler should skip without error when CRDs are missing")
	})

	t.Run("OPAReconciler", func(t *testing.T) {
		r := &OPAReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme: scheme,
		}
		err := r.ReconcileOPA(ctx, "default", DefaultOPAConfig())
		assert.NoError(t, err, "OPAReconciler should skip without error when CRDs are missing")
	})

	t.Run("EbpfReconciler", func(t *testing.T) {
		r := &EbpfReconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			Scheme: scheme,
		}
		svc := minimalLLMSvc("test-svc", "default")
		err := r.ReconcileEbpfPolicy(ctx, svc)
		assert.NoError(t, err, "EbpfReconciler should skip without error when CRDs are missing")
	})
}
