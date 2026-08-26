package deployment

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

func (b *Builder) ensureVLLMEnv(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	modelID := llmSvc.Spec.Model.Name
	if modelID == "" {
		modelID = strings.ReplaceAll(llmSvc.Spec.Model.URI, "/", ".")
	}
	engine := llmSvc.Spec.Engine
	if engine == "" {
		engine = "vllm"
	}
	envs := map[string]string{"HOME": "/tmp", "VLLM_TARGET_DEVICE": "cpu", "USER": "nonroot", "LOGNAME": "nonroot", "TORCHINDUCTOR_CACHE_DIR": "/tmp", "VLLM_LOGGING_LEVEL": "INFO", "OIS_MODEL_ID": modelID, "OIS_MODEL_URN": observability.URN("model", modelID), "OIS_ENGINE_URN": observability.URN("engine", engine), "OIS_ACTOR_URN": observability.URN("actor", llmSvc.Namespace)}
	for name, value := range envs {
		setEnvDefault(&podSpec.Containers[0], name, value)
	}
}

func (b *Builder) injectVector(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec) {
	sink := "stdout"
	if b.OTEL_Endpoint != "" {
		sink = "otlp"
	}
	if llmSvc.Spec.Observability != nil && llmSvc.Spec.Observability.Sink != nil {
		sink = llmSvc.Spec.Observability.Sink.Type
	}
	if sink != "stdout" || b.OTEL_Endpoint != "" {
		observability.InjectVectorSidecar(podSpec, llmSvc.Name+"-vector-config")
	}
}

func (b *Builder) buildAnnotations(llmSvc *servingv1alpha2.LLMInferenceService, podSpec *corev1.PodSpec, kvRole string) map[string]string {
	ann := map[string]string{}
	if len(podSpec.Containers) > 0 {
		ann["serving.ckodex.com/runtime-image"] = podSpec.Containers[0].Image
	}
	if llmSvc.Spec.KVCache != nil && llmSvc.Spec.KVCache.Transfer != nil {
		b.addKVAnnotations(ann, llmSvc, kvRole)
	}
	if llmSvc.Spec.Prefill != nil {
		ann["serving.ckodex.com/pd-disaggregation"] = "true"
	}
	if llmSvc.Spec.Canary != nil {
		ann["ckodex.com/canary-weight"] = fmt.Sprintf("%d", llmSvc.Spec.Canary.Weight)
	}
	if toolSurfaceEnabled(llmSvc) {
		ann["sidecar.istio.io/inject"] = "true"
		ann["sidecar.istio.io/rewriteAppHTTPProbers"] = "true"
		ann["sidecar.istio.io/discoveryNamespaces"] = llmSvc.Namespace
	}
	return ann
}

func (b *Builder) addKVAnnotations(ann map[string]string, llmSvc *servingv1alpha2.LLMInferenceService, role string) {
	transfer := llmSvc.Spec.KVCache.Transfer
	ann["serving.ckodex.com/kv-connector"] = transfer.Connector
	ann["serving.ckodex.com/kv-role"] = transferRole(transfer.Role, role)
}

func toolSurfaceEnabled(llmSvc *servingv1alpha2.LLMInferenceService) bool {
	return llmSvc.Spec.ToolSurface != nil && (len(llmSvc.Spec.ToolSurface.AllowedAPIs) > 0 || len(llmSvc.Spec.ToolSurface.AllowedCIDRs) > 0)
}
