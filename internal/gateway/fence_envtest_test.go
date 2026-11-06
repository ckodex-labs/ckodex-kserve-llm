package gateway

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	serving "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	gateway "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

// These tests exercise reads and optimistic-lock patches against a real API
// server using the dependency-pinned Gateway API CRDs. Data-plane behavior is
// verified separately.
func TestFenceOwnedRoutesAPI(t *testing.T) {
	assets := os.Getenv("GATEWAY_ENVTEST_ASSETS")
	if assets == "" {
		if os.Getenv("REQUIRE_GATEWAY_ENVTEST") == "1" {
			t.Fatal("GATEWAY_ENVTEST_ASSETS is required")
		}
		t.Skip("set GATEWAY_ENVTEST_ASSETS to run real API-server tests")
	}
	t.Setenv("ETCD_MAX_REQUEST_BYTES", "52428800")
	env := &envtest.Environment{BinaryAssetsDirectory: assets, CRDs: gatewayCRDs(t)}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start API server: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stop API server: %v", err)
		}
	})
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := serving.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := gateway.Install(scheme); err != nil {
		t.Fatal(err)
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "fence-"}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	service := &serving.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: ns.Name}, Spec: serving.LLMInferenceServiceSpec{Model: serving.ModelSpec{Name: "model", URI: "hf://example/model"}, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "runtime", Image: "example/runtime"}}}}}}
	if err := c.Create(ctx, service); err != nil {
		t.Fatalf("create service: %v", err)
	}
	foreign := service.DeepCopy()
	foreign.Name = "other"
	foreign.ResourceVersion = ""
	foreign.UID = ""
	if err := c.Create(ctx, foreign); err != nil {
		t.Fatalf("create foreign service: %v", err)
	}
	if service.UID == "" || foreign.UID == "" || service.UID == foreign.UID {
		t.Fatal("API server did not assign distinct service UIDs")
	}
	if err := FenceOwnedRoutes(ctx, c, c, service); err != nil {
		t.Fatalf("missing routes: %v", err)
	}
	if err := FenceOwnedRoutes(ctx, nil, c, service); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := FenceOwnedRoutes(ctx, c, nil, service); err == nil {
		t.Fatal("nil writer accepted")
	}
	if err := FenceOwnedRoutes(ctx, c, c, nil); err == nil {
		t.Fatal("nil service accepted")
	}
	if err := FenceOwnedRoutes(ctx, c, c, &serving.LLMInferenceService{}); err == nil {
		t.Fatal("empty UID accepted")
	}
	controller := true
	owner := metav1.OwnerReference{APIVersion: "serving.ckodex.com/v1alpha2", Kind: "LLMInferenceService", Name: service.Name, UID: service.UID, Controller: &controller}
	http := &gateway.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "model-httproute", Namespace: ns.Name, OwnerReferences: []metav1.OwnerReference{owner}}, Spec: gateway.HTTPRouteSpec{Rules: []gateway.HTTPRouteRule{{BackendRefs: []gateway.HTTPBackendRef{{BackendRef: gateway.BackendRef{BackendObjectReference: gateway.BackendObjectReference{Name: "backend", Port: func() *gateway.PortNumber { p := gateway.PortNumber(8080); return &p }()}}}}}}}}
	grpc := &gateway.GRPCRoute{ObjectMeta: metav1.ObjectMeta{Name: "model-grpcroute", Namespace: ns.Name, OwnerReferences: []metav1.OwnerReference{owner}}, Spec: gateway.GRPCRouteSpec{Rules: []gateway.GRPCRouteRule{{BackendRefs: []gateway.GRPCBackendRef{{BackendRef: gateway.BackendRef{BackendObjectReference: gateway.BackendObjectReference{Name: "backend", Port: func() *gateway.PortNumber { p := gateway.PortNumber(8080); return &p }()}}}}}}}}
	if err := c.Create(ctx, http); err != nil {
		t.Fatal(err)
	}
	if err := c.Create(ctx, grpc); err != nil {
		t.Fatal(err)
	}
	// Foreign HTTP ownership must not prevent the owned gRPC route from fencing.
	http.OwnerReferences[0].UID = foreign.UID
	http.OwnerReferences[0].Name = foreign.Name
	if err := c.Update(ctx, http); err != nil {
		t.Fatal(err)
	}
	if err := FenceOwnedRoutes(ctx, c, c, service); err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("foreign ownership: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(grpc), grpc); err != nil {
		t.Fatal(err)
	}
	if len(grpc.Spec.Rules[0].BackendRefs) != 0 {
		t.Fatal("gRPC route still forwards")
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(http), http); err != nil {
		t.Fatal(err)
	}
	if len(http.Spec.Rules[0].BackendRefs) != 1 {
		t.Fatal("foreign HTTP route was modified")
	}
	http.OwnerReferences[0].UID = service.UID
	http.OwnerReferences[0].Name = service.Name
	if err := c.Update(ctx, http); err != nil {
		t.Fatal(err)
	}
	if err := FenceOwnedRoutes(ctx, c, c, service); err != nil {
		t.Fatalf("fence owned routes: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(http), http); err != nil {
		t.Fatal(err)
	}
	if len(http.Spec.Rules[0].BackendRefs) != 0 {
		t.Fatal("HTTP route still forwards")
	}
	if err := FenceOwnedRoutes(ctx, c, c, service); err != nil {
		t.Fatalf("repeat fencing: %v", err)
	}
	// Reverse ownership exercises the independent HTTP attempt as well.
	port := gateway.PortNumber(8080)
	http.Spec.Rules[0].BackendRefs = []gateway.HTTPBackendRef{{BackendRef: gateway.BackendRef{BackendObjectReference: gateway.BackendObjectReference{Name: "backend", Port: &port}}}}
	if err := c.Update(ctx, http); err != nil {
		t.Fatal(err)
	}
	grpc.OwnerReferences[0].UID = foreign.UID
	grpc.OwnerReferences[0].Name = foreign.Name
	if err := c.Update(ctx, grpc); err != nil {
		t.Fatal(err)
	}
	if err := FenceOwnedRoutes(ctx, c, c, service); err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("foreign gRPC ownership: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(http), http); err != nil {
		t.Fatal(err)
	}
	if len(http.Spec.Rules[0].BackendRefs) != 0 {
		t.Fatal("HTTP route still forwards after gRPC ownership failure")
	}
}

func gatewayCRDs(t *testing.T) []*apiextensions.CustomResourceDefinition {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/gateway-api").Output()
	if err != nil {
		t.Fatalf("locate Gateway API CRDs: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "config", "crd", "standard")
	var crds []*apiextensions.CustomResourceDefinition
	for _, kind := range []string{"httproutes", "grpcroutes"} {
		data, err := os.ReadFile(filepath.Join(dir, "gateway.networking.k8s.io_"+kind+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var crd apiextensions.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &crd); err != nil {
			t.Fatal(err)
		}
		crds = append(crds, &crd)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "serving.ckodex.com_llminferenceservices.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var serviceCRD apiextensions.CustomResourceDefinition
	if err := yaml.Unmarshal(data, &serviceCRD); err != nil {
		t.Fatal(err)
	}
	crds = append(crds, &serviceCRD)
	return crds
}
