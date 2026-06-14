package aipack

import v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"

// Manifest media types per AIPACK-SPEC v0.1.1 §4.1.
const (
	MediaTypeBaseModel      = "application/vnd.ai.basemodel.v1+json"
	MediaTypeLoRA           = "application/vnd.ai.lora.v1+json"
	MediaTypeFineTune       = "application/vnd.ai.finetune.v1+json"
	MediaTypeSkill          = "application/vnd.ai.skill.v1+json"
	MediaTypeTool           = "application/vnd.ai.tool.v1+json"
	MediaTypeMCPServer      = "application/vnd.ai.mcp-server.v1+json"
	MediaTypePromptTemplate = "application/vnd.ai.prompt-template.v1+json"
	MediaTypeGuardrail      = "application/vnd.ai.guardrail.v1+json"
	MediaTypeRetrievalIndex = "application/vnd.ai.retrieval.v1+json"
	MediaTypeDataset        = "application/vnd.ai.dataset.v1+json"
	MediaTypeHarness        = "application/vnd.ai.harness.v1+json"
	MediaTypeEval           = "application/vnd.ai.eval.v1+json"
	MediaTypeWorkflow       = "application/vnd.ai.workflow.v1+json"
	MediaTypePolicyBundle   = "application/vnd.ai.policy-bundle.v1+json"
	MediaTypeAgent          = "application/vnd.ai.agent.v1+json"
)

// Content layer media types per AIPACK-SPEC v0.1.1 §4.2.
const (
	LayerBaseModelWeights = "application/vnd.ai.basemodel.weights.v1.safetensors"
	LayerLoRAWeights      = "application/vnd.ai.lora.weights.v1.safetensors"
	LayerFineTuneWeights  = "application/vnd.ai.finetune.weights.v1.safetensors"
	LayerSkillBundle      = "application/vnd.ai.skill.bundle.v1.tar+zstd"
	LayerGuardrailRules   = "application/vnd.ai.guardrail.rules.v1.tar+zstd"
	LayerRetrievalIndex   = "application/vnd.ai.retrieval.index.v1.tar+zstd"
	LayerDatasetShard     = "application/vnd.ai.dataset.shard.v1.tar+zstd"
	LayerHarnessBundle    = "application/vnd.ai.harness.v1.tar+zstd"
	LayerAirGapBundle     = "application/vnd.ai.aipack.airgap-bundle.v1.tar+zstd"
)

// kindToMediaType is the authoritative kind→manifest media type lookup per §4.1.
var kindToMediaType = map[v1alpha2.ArtifactKind]string{
	v1alpha2.KindBaseModel:      MediaTypeBaseModel,
	v1alpha2.KindLoRA:           MediaTypeLoRA,
	v1alpha2.KindFineTune:       MediaTypeFineTune,
	v1alpha2.KindSkill:          MediaTypeSkill,
	v1alpha2.KindTool:           MediaTypeTool,
	v1alpha2.KindMCPServer:      MediaTypeMCPServer,
	v1alpha2.KindPromptTemplate: MediaTypePromptTemplate,
	v1alpha2.KindGuardrail:      MediaTypeGuardrail,
	v1alpha2.KindRetrievalIndex: MediaTypeRetrievalIndex,
	v1alpha2.KindDataset:        MediaTypeDataset,
	v1alpha2.KindHarness:        MediaTypeHarness,
	v1alpha2.KindEval:           MediaTypeEval,
	v1alpha2.KindWorkflow:       MediaTypeWorkflow,
	v1alpha2.KindPolicyBundle:   MediaTypePolicyBundle,
	v1alpha2.KindAgent:          MediaTypeAgent,
}

// MediaTypeForKind returns the manifest media type for the given kind.
// Returns ("", false) when the kind is unknown (AIPACK-KIND-000).
func MediaTypeForKind(kind v1alpha2.ArtifactKind) (string, bool) {
	mt, ok := kindToMediaType[kind]
	return mt, ok
}

// ValidateMediaType returns an error if the declared media type does not match
// the canonical type for the given kind (AIPACK-RUNTIME-001).
func ValidateMediaType(kind v1alpha2.ArtifactKind, declared string) error {
	canonical, ok := kindToMediaType[kind]
	if !ok {
		return newErr(ErrKindUnknown, "unknown artifact kind", string(kind))
	}
	if declared != "" && declared != canonical {
		return newErr(ErrMediaTypeMismatch,
			"declared mediaType does not match §4.1 canonical media type",
			string(kind)+": got "+declared+", want "+canonical,
		)
	}
	return nil
}
