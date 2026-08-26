package deployment

import (
	"context"
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestBuilderUsesVLLMAdapterForRuntimeArguments(t *testing.T) {
	service := baseQuantLLMSvc("runtime-adapter")
	service.Spec.Parallelism = &servingv1alpha2.ParallelismSpec{
		Tensor: ptr.To(int32(2)), Pipeline: ptr.To(int32(3)), Expert: true,
	}
	service.Spec.KVCache = &servingv1alpha2.KVCacheSpec{
		Dtype: "fp8", SwapSpaceGB: ptr.To(int32(6)),
	}
	service.Spec.SpeculativeDecoding = &servingv1alpha2.SpeculativeDecodingSpec{
		Method: "mtp", NumTokens: ptr.To(int32(4)),
	}

	deployment := builderForQuantTest(t).Build(context.Background(), service, 1, HardwareNVIDIA, nil)
	require.NotNil(t, deployment)
	args := deployment.Spec.Template.Spec.Containers[0].Args
	assertContainsArgPair(t, args, "--tensor-parallel-size", "2")
	assertContainsArgPair(t, args, "--pipeline-parallel-size", "3")
	assertContainsArgPair(t, args, "--kv-cache-dtype", "fp8")
	assertContainsArgPair(t, args, "--cpu-offload-gb", "6")
	assertContainsArgPair(t, args, "--spec-method", "mtp")
	assertContainsArgPair(t, args, "--spec-tokens", "4")
	require.Contains(t, args, "--enable-expert-parallel")
}
