package reconciler

import (
	"context"
	"testing"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPDBReconcilerCreatesAndConverges(t *testing.T) {
	scheme := serviceTestScheme(t)
	llm := serviceTestLLM()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &PDBReconciler{Client: cl, Scheme: scheme}
	if err := r.Reconcile(context.Background(), llm); err != nil {
		t.Fatal(err)
	}
	var pdb policyv1.PodDisruptionBudget
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "model", Namespace: "models"}, &pdb); err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.IntValue() != 1 {
		t.Fatalf("unexpected PDB: %#v", pdb.Spec)
	}
	if err := r.Reconcile(context.Background(), llm); err != nil {
		t.Fatal(err)
	}
}

func TestPDBReconcilerUpdatesSpec(t *testing.T) {
	scheme := serviceTestScheme(t)
	llm := serviceTestLLM()
	pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"}, Spec: policyv1.PodDisruptionBudgetSpec{}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llm, pdb).Build()
	if err := (&PDBReconciler{Client: cl, Scheme: scheme}).Reconcile(context.Background(), llm); err != nil {
		t.Fatal(err)
	}
	var got policyv1.PodDisruptionBudget
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "model", Namespace: "models"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.MinAvailable == nil || got.Spec.Selector == nil {
		t.Fatalf("PDB spec was not reconciled: %#v", got.Spec)
	}
}
