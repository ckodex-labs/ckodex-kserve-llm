package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestCapabilityMatrixDeclaresEveryCapability(t *testing.T) {
	matrix := CapabilityMatrix{
		TensorParallel: CapabilitySupported, DataParallel: CapabilitySupported,
		LocalDataParallel: CapabilitySupported, PipelineParallel: CapabilitySupported,
		ExpertParallel: CapabilitySupported, ExpertLoadBalancing: CapabilitySupported,
		KVCacheDtype: CapabilitySupported, CPUOffload: CapabilitySupported,
		KVTransfer: CapabilityEmulated, SpeculativeDecoding: CapabilitySupported,
		Quantization: CapabilitySupported, LoRAHotSwap: CapabilityEmulated,
	}

	value := reflect.ValueOf(matrix)
	for index := 0; index < value.NumField(); index++ {
		if value.Field(index).String() == "" {
			t.Fatalf("capability %s is undeclared", value.Type().Field(index).Name)
		}
	}
}

func TestImageContractRequiresCompleteSHA256Reference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	contract := ImageContract{Repository: "registry.example/runtime", Tag: "v1.2.3", Digest: digest}
	if !contract.Valid() {
		t.Fatal("complete image contract was rejected")
	}
	if got, want := contract.Reference(), "registry.example/runtime:v1.2.3@"+digest; got != want {
		t.Fatalf("Reference() = %q, want %q", got, want)
	}

	for _, invalid := range []ImageContract{
		{},
		{Repository: contract.Repository, Tag: contract.Tag, Digest: "sha256:short"},
		{Repository: contract.Repository, Tag: contract.Tag, Digest: strings.Repeat("a", 64)},
		{Repository: contract.Repository, Tag: contract.Tag, Digest: "sha256:" + strings.Repeat("z", 64)},
	} {
		if invalid.Valid() {
			t.Fatalf("invalid image contract accepted: %+v", invalid)
		}
	}
}
