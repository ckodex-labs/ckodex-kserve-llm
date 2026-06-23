package aipack

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"

	v1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// digestRefPattern matches OCI digest references of the form <registry>/<repo>@sha256:<64hex>.
var digestRefPattern = regexp.MustCompile(`^.+@sha256:[0-9a-f]{64}$`)

// AgentSlotConstraints declares which artifact kinds are valid in each Agent slot per §5.3.
var AgentSlotConstraints = map[string][]v1alpha2.ArtifactKind{
	"baseModel":        {v1alpha2.KindBaseModel, v1alpha2.KindLoRA, v1alpha2.KindFineTune},
	"adapters":         {v1alpha2.KindLoRA},
	"skills":           {v1alpha2.KindSkill},
	"tools":            {v1alpha2.KindTool, v1alpha2.KindMCPServer},
	"mcpServers":       {v1alpha2.KindMCPServer},
	"systemPrompt":     {v1alpha2.KindPromptTemplate},
	"guardrailsInput":  {v1alpha2.KindGuardrail},
	"guardrailsOutput": {v1alpha2.KindGuardrail},
	"retrieval":        {v1alpha2.KindRetrievalIndex},
	"workflow":         {v1alpha2.KindWorkflow},
	"policy":           {v1alpha2.KindPolicyBundle},
}

// ValidateRef returns AIPACK-COMP-001 when ref does not include a sha256 digest.
// Tag-only references are rejected to ensure immutable artifact identity.
func ValidateRef(ref string) error {
	if !digestRefPattern.MatchString(ref) {
		return newErr(ErrTagOnlyRef,
			"OCI reference must include sha256 digest (tag-only refs rejected per AIPACK-COMP-001)",
			ref,
		)
	}
	return nil
}

// ValidateComposition validates all composition refs and slot constraints for an Agent artifact.
// Returns the first error encountered.
func ValidateComposition(comp *v1alpha2.AIPackComposition) error {
	if comp == nil {
		return nil
	}
	if comp.BaseModel != nil {
		if err := ValidateRef(comp.BaseModel.Ref); err != nil {
			return err
		}
		if err := validateSlotKind("baseModel", comp.BaseModel.Kind); err != nil {
			return err
		}
	}
	for i, r := range comp.Adapters {
		if err := ValidateRef(r.Ref); err != nil {
			return fmt.Errorf("adapters[%d]: %w", i, err)
		}
		if err := validateSlotKind("adapters", r.Kind); err != nil {
			return fmt.Errorf("adapters[%d]: %w", i, err)
		}
	}
	for i, r := range comp.Skills {
		if err := ValidateRef(r.Ref); err != nil {
			return fmt.Errorf("skills[%d]: %w", i, err)
		}
		if err := validateSlotKind("skills", r.Kind); err != nil {
			return fmt.Errorf("skills[%d]: %w", i, err)
		}
	}
	for i, r := range comp.Tools {
		if err := ValidateRef(r.Ref); err != nil {
			return fmt.Errorf("tools[%d]: %w", i, err)
		}
		if err := validateSlotKind("tools", r.Kind); err != nil {
			return fmt.Errorf("tools[%d]: %w", i, err)
		}
	}
	for i, r := range comp.MCPServers {
		if err := ValidateRef(r.Ref); err != nil {
			return fmt.Errorf("mcpServers[%d]: %w", i, err)
		}
		if err := validateSlotKind("mcpServers", r.Kind); err != nil {
			return fmt.Errorf("mcpServers[%d]: %w", i, err)
		}
	}
	for _, r := range singleSlots(comp) {
		if r.ref == nil {
			continue
		}
		if err := ValidateRef(r.ref.Ref); err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
		if err := validateSlotKind(r.name, r.ref.Kind); err != nil {
			return fmt.Errorf("%s: %w", r.name, err)
		}
	}
	return nil
}

type singleSlot struct {
	name string
	ref  *v1alpha2.AIPackRef
}

func singleSlots(comp *v1alpha2.AIPackComposition) []singleSlot {
	return []singleSlot{
		{"systemPrompt", comp.SystemPrompt},
		{"retrieval", comp.Retrieval},
		{"workflow", comp.Workflow},
		{"policy", comp.Policy},
	}
}

func validateSlotKind(slot string, kind v1alpha2.ArtifactKind) error {
	if kind == "" {
		return nil // Kind not declared; skip slot type check
	}
	allowed, ok := AgentSlotConstraints[slot]
	if !ok {
		return nil // Unknown slot; skip (forward-compatible)
	}
	for _, a := range allowed {
		if kind == a {
			return nil
		}
	}
	return newErr(ErrSlotTypeMismatch,
		"artifact kind not permitted in slot",
		fmt.Sprintf("slot=%s kind=%s allowed=%v", slot, kind, allowed),
	)
}

// CheckLoRACompatibility returns AIPACK-COMPAT-001 when lora.BaseRef is absent.
func CheckLoRACompatibility(lora *v1alpha2.LoRASpec) error {
	if lora == nil {
		return nil
	}
	if lora.BaseRef == "" {
		return newErr(ErrLoRABaseRefMissing,
			"LoRA artifact must declare baseRef pointing to the target BaseModel (AIPACK-COMPAT-001)",
			"",
		)
	}
	return ValidateRef(lora.BaseRef)
}

// CheckRetrievalCompatibility returns AIPACK-COMPAT-002 when embeddingModel is absent.
func CheckRetrievalCompatibility(idx *v1alpha2.RetrievalIndexSpec) error {
	if idx == nil {
		return nil
	}
	if idx.EmbeddingModel == "" {
		return newErr(ErrRetrievalEmbedMismatch,
			"RetrievalIndex must declare embeddingModel ref (AIPACK-COMPAT-002)",
			"",
		)
	}
	return ValidateRef(idx.EmbeddingModel)
}

// ComputeCompositeDigest computes the RFC 8785 canonical JSON sha256 of the composition.
// The digest is used as the CompositeDigest field on AIPackComposition.
// RFC 8785: keys sorted alphabetically, no insignificant whitespace.
// Note: encoding/json in Go produces alphabetically sorted keys for struct fields,
// which approximates RFC 8785 for the flat types used here.
func ComputeCompositeDigest(comp *v1alpha2.AIPackComposition) (string, error) {
	// TODO(ckodex): replace with a full RFC 8785 canonical JSON implementation
	// when the joeshaw/canonicaljson or similar library is vendored.
	b, err := json.Marshal(comp)
	if err != nil {
		return "", fmt.Errorf("marshal composition: %w", err)
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum), nil
}
