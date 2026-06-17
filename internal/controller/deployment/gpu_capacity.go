package deployment

import (
	"fmt"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// CheckGPURequirements verifies if the cluster has enough GPU capacity for the service.
// Returns (bool, string) where bool is true if requirements are met.
func CheckGPURequirements(llmSvc *servingv1alpha2.LLMInferenceService, totalGpus int) (bool, string) {
	reqGpus := 0

	// Convention: main container is at index 0.
	if len(llmSvc.Spec.Template.Spec.Containers) > 0 {
		res := llmSvc.Spec.Template.Spec.Containers[0].Resources
		if g, ok := res.Requests["nvidia.com/gpu"]; ok {
			val, _ := g.AsInt64()
			reqGpus = int(val)
		}
	}

	if reqGpus > totalGpus {
		return false, fmt.Sprintf("Requested %d GPUs but only %d available in cluster", reqGpus, totalGpus)
	}
	return true, ""
}
