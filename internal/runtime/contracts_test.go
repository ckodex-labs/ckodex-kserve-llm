package runtime

import (
	"reflect"
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
