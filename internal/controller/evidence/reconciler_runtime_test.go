package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/ckodex-labs/kserve-llm-operator/internal/provenance"
)

type listErrorClient struct{ client.Client }

func (listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("pod list unavailable")
}

func readyPod(name, message string, ready bool) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{
		"app.kubernetes.io/name": "llminferenceservice", "app.kubernetes.io/instance": "runtime-svc",
	}}}
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "storage-initializer", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: message}}}}
	return pod
}

func validVerificationMessage(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(provenance.RuntimeVerificationRecord{
		SignatureVerified: true, AttestationVerified: true, SBOMVerified: true,
		SignatureDigest: "sha256:s", AttestationURI: "oci://model#attestation", SBOMDigest: "sha256:b",
	})
	if err != nil {
		t.Fatalf("marshal verification record: %v", err)
	}
	return string(b)
}

func TestReconcileRuntime_VerifiesReadyOCIPod(t *testing.T) {
	svc := baseLLMSvcForEvidence("runtime-svc")
	svc.Spec.Model.URI = "ocis://registry/model@sha256:abc"
	scheme := evidenceTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyPod("ready", validVerificationMessage(t), true)).Build()
	if err := (&GovernanceReconciler{Client: c}).Reconcile(context.Background(), svc, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := condition(t, svc, "Compliance-SR-2"); got.Reason != "ProvenanceVerified" {
		t.Fatalf("SR-2 reason = %q, want ProvenanceVerified", got.Reason)
	}
}

func TestReconcileRuntime_RejectsInvalidRecordAndPropagatesListError(t *testing.T) {
	tests := []struct {
		name string
		cl   client.Client
		want string
	}{
		{name: "invalid record", cl: fake.NewClientBuilder().WithScheme(evidenceTestScheme(t)).WithObjects(readyPod("bad", "not-json", true)).Build(), want: "parse init-container verification record"},
		{name: "client list", cl: listErrorClient{}, want: "inspect base model verification"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := baseLLMSvcForEvidence("runtime-svc")
			svc.Spec.Model.URI = "oci://registry/model@sha256:abc"
			err := (&GovernanceReconciler{Client: tt.cl}).Reconcile(context.Background(), svc, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
