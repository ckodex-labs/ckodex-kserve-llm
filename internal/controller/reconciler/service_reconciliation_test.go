package reconciler

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func serviceTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := servingv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func serviceTestLLM() *servingv1alpha2.LLMInferenceService {
	return &servingv1alpha2.LLMInferenceService{
		TypeMeta:   metav1.TypeMeta{APIVersion: servingv1alpha2.GroupVersion, Kind: "LLMInferenceService"},
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models", UID: "model-uid"},
		Spec:       servingv1alpha2.LLMInferenceServiceSpec{Model: servingv1alpha2.ModelSpec{Name: "gemma"}},
	}
}

func TestServiceReconcilerCreatesClusterAndHeadlessServices(t *testing.T) {
	scheme := serviceTestScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	llm := serviceTestLLM()
	r := &ServiceReconciler{Client: cl, Scheme: scheme, EnableGRPC: true}

	if err := r.Reconcile(context.Background(), llm); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"model", "model-headless"} {
		var svc corev1.Service
		if err := cl.Get(context.Background(), client.ObjectKey{Name: name, Namespace: "models"}, &svc); err != nil {
			t.Fatal(err)
		}
		if len(svc.Spec.Ports) != 2 || svc.Spec.Ports[1].Name != "grpc-inference" {
			t.Fatalf("unexpected ports for %s: %#v", name, svc.Spec.Ports)
		}
	}
}

func TestServiceReconcilerUpdatesOnlyManagedServiceFields(t *testing.T) {
	scheme := serviceTestScheme(t)
	llm := serviceTestLLM()
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"}, Spec: corev1.ServiceSpec{
		Selector: map[string]string{"old": "selector"}, Ports: []corev1.ServicePort{{Name: "old", Port: 9}}, ClusterIP: "10.0.0.1",
	}}
	headless := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "model-headless", Namespace: "models"}, Spec: corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(llm, existing, headless).Build()
	if err := (&ServiceReconciler{Client: cl, Scheme: scheme}).Reconcile(context.Background(), llm); err != nil {
		t.Fatal(err)
	}
	var got corev1.Service
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "model", Namespace: "models"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.ClusterIP != "10.0.0.1" || got.Spec.Ports[0].Port != 80 {
		t.Fatalf("immutable or desired service fields not preserved: %#v", got.Spec)
	}
}
