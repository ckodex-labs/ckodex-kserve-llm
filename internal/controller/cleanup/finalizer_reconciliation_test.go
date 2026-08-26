package cleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func cleanupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func cleanupObject() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"}}
}

func TestFinalizerReconcilerAddsFinalizerOnce(t *testing.T) {
	obj := cleanupObject()
	cl := fake.NewClientBuilder().WithScheme(cleanupTestScheme(t)).WithObjects(obj).Build()
	done, err := (&Reconciler{Client: cl}).HandleFinalizer(context.Background(), obj, "cleanup.ckodex.com", nil)
	if err != nil || done || len(obj.Finalizers) != 1 {
		t.Fatalf("add finalizer: done=%v err=%v finalizers=%v", done, err, obj.Finalizers)
	}
	done, err = (&Reconciler{Client: cl}).HandleFinalizer(context.Background(), obj, "cleanup.ckodex.com", nil)
	if err != nil || done || len(obj.Finalizers) != 1 {
		t.Fatalf("existing finalizer: done=%v err=%v finalizers=%v", done, err, obj.Finalizers)
	}
}

func TestFinalizerReconcilerCleansAndRemovesFinalizer(t *testing.T) {
	obj := cleanupObject()
	obj.Finalizers = []string{"cleanup.ckodex.com"}
	deletionTime := metav1.NewTime(time.Now())
	obj.DeletionTimestamp = &deletionTime
	cl := fake.NewClientBuilder().WithScheme(cleanupTestScheme(t)).WithObjects(obj).Build()
	called := false
	done, err := (&Reconciler{Client: cl}).HandleFinalizer(context.Background(), obj, "cleanup.ckodex.com", func() error { called = true; return nil })
	if err != nil || !done || !called || len(obj.Finalizers) != 0 {
		t.Fatalf("cleanup: done=%v err=%v called=%v finalizers=%v", done, err, called, obj.Finalizers)
	}
}

func TestFinalizerReconcilerStopsWhenCleanupFails(t *testing.T) {
	obj := cleanupObject()
	obj.Finalizers = []string{"cleanup.ckodex.com"}
	deletionTime := metav1.NewTime(time.Now())
	obj.DeletionTimestamp = &deletionTime
	want := errors.New("dependency unavailable")
	cl := fake.NewClientBuilder().WithScheme(cleanupTestScheme(t)).WithObjects(obj).Build()
	done, err := (&Reconciler{Client: cl}).HandleFinalizer(context.Background(), obj, "cleanup.ckodex.com", func() error { return want })
	if !errors.Is(err, want) || done || len(obj.Finalizers) != 1 {
		t.Fatalf("cleanup failure: done=%v err=%v finalizers=%v", done, err, obj.Finalizers)
	}
}
