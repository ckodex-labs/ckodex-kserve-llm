package status

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func statusTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func statusTestLLM() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models", Generation: 4},
		Spec:       servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{Name: "gemma", Revision: "r2"}},
	}
}

func TestStatusReconcilerUpdateHandlesMissingDeployment(t *testing.T) {
	scheme := statusTestScheme(t)
	llm := statusTestLLM()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(llm).WithObjects(llm).Build()
	current := llm.DeepCopy()
	if err := (&Reconciler{Client: cl, EnableHardening: true}).Update(context.Background(), current, llm, false, nil); err != nil {
		t.Fatal(err)
	}
	if current.Status.ModelReady || current.Status.Replicas != 0 {
		t.Fatalf("missing deployment should be not ready: %#v", current.Status)
	}
	if condition := findCondition(current, servingv1alpha2.ConditionDeploymentReady); condition == nil || condition.Reason != "DeploymentUnavailable" {
		t.Fatalf("missing deployment condition not recorded: %#v", current.Status.Conditions)
	}
}

func TestStatusReconcilerUpdatePrefillStatesAndPreservesTransition(t *testing.T) {
	scheme := statusTestScheme(t)
	llm := statusTestLLM()
	replicas := int32(2)
	llm.Spec.Prefill = &servingv1alpha2.PrefillSpec{Replicas: &replicas}
	llm.Spec.KVCache = &servingv1alpha2.KVCacheSpec{Transfer: &servingv1alpha2.KVTransferSpec{Connector: "lmcache"}}
	primary := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}
	prefill := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "model-prefill", Namespace: "models"}, Status: appsv1.DeploymentStatus{ReadyReplicas: 2}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(llm).WithObjects(llm, primary, prefill).Build()
	r := &Reconciler{Client: cl, EnableHardening: true}
	current := llm.DeepCopy()
	if err := r.Update(context.Background(), current, llm, true, nil); err != nil {
		t.Fatal(err)
	}
	ready := findCondition(current, servingv1alpha2.ConditionReady)
	if ready == nil || ready.Status != metav1.ConditionTrue || findCondition(current, servingv1alpha2.ConditionPrefillReady).Status != metav1.ConditionTrue {
		t.Fatalf("ready conditions not projected: %#v", current.Status.Conditions)
	}
	transition := ready.LastTransitionTime
	if err := r.Update(context.Background(), current, current.DeepCopy(), true, nil); err != nil {
		t.Fatal(err)
	}
	if got := findCondition(current, servingv1alpha2.ConditionReady).LastTransitionTime; !got.Equal(&transition) {
		t.Fatalf("stable condition transition changed: before=%v after=%v", transition, got)
	}
}

func TestStatusReconcilerUpdateFromKServeFallbacks(t *testing.T) {
	scheme := statusTestScheme(t)
	llm := statusTestLLM()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(llm).WithObjects(llm).Build()
	current := llm.DeepCopy()
	if err := (&Reconciler{Client: cl}).UpdateFromKServe(context.Background(), current, llm, false, nil); err != nil {
		t.Fatal(err)
	}
	if current.Status.ModelReady || current.Status.Replicas != 0 || current.Status.URL == "" {
		t.Fatalf("KServe fallback status incomplete: %#v", current.Status)
	}
}

func TestStatusReconcilerSetConditionPatchesObject(t *testing.T) {
	scheme := statusTestScheme(t)
	llm := statusTestLLM()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(llm).WithObjects(llm).Build()
	if err := (&Reconciler{Client: cl}).SetCondition(context.Background(), llm, "GPUCapacity", metav1.ConditionTrue, "Available", "GPU is available"); err != nil {
		t.Fatal(err)
	}
	var got servingv1alpha2.LLMInferenceService
	if err := cl.Get(context.Background(), types.NamespacedName{Name: llm.Name, Namespace: llm.Namespace}, &got); err != nil {
		t.Fatal(err)
	}
	if findCondition(&got, "GPUCapacity") == nil {
		t.Fatal("patched condition was not persisted")
	}
}

func findCondition(llm *servingv1alpha2.LLMInferenceService, conditionType string) *metav1.Condition {
	for i := range llm.Status.Conditions {
		if llm.Status.Conditions[i].Type == conditionType {
			return &llm.Status.Conditions[i]
		}
	}
	return nil
}
