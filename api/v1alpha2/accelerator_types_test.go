/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	"testing"
)

func TestAcceleratorType_VendorAndModel(t *testing.T) {
	tests := []struct {
		input      AcceleratorType
		wantVendor AcceleratorType
		wantModel  AcceleratorModel
	}{
		// NVIDIA
		{input: "nvidia", wantVendor: AcceleratorTypeNVIDIA, wantModel: ""},
		{input: "nvidia/rtx-pro-6000", wantVendor: AcceleratorTypeNVIDIA, wantModel: AcceleratorModelRTXPRO6000},
		{input: "nvidia-sm100", wantVendor: AcceleratorTypeNVIDIA, wantModel: AcceleratorModelSM100},
		{input: "nvidia/gb300", wantVendor: AcceleratorTypeNVIDIA, wantModel: AcceleratorModelGB300},
		{input: "nvidia-gb300", wantVendor: AcceleratorTypeNVIDIA, wantModel: AcceleratorModelGB300},

		// ROCm
		{input: "rocm", wantVendor: AcceleratorTypeROCm, wantModel: ""},
		{input: "rocm/mi300x", wantVendor: AcceleratorTypeROCm, wantModel: AcceleratorModelMI300X},
		{input: "rocm-mi325x", wantVendor: AcceleratorTypeROCm, wantModel: AcceleratorModelMI325X},

		// Apple
		{input: "apple", wantVendor: AcceleratorTypeAppleMetal, wantModel: ""},
		{input: "apple-metal", wantVendor: AcceleratorTypeAppleMetal, wantModel: ""},
		{input: "apple-mlx", wantVendor: AcceleratorTypeAppleMLX, wantModel: ""},
		{input: "apple-metal/m4-ultra", wantVendor: AcceleratorTypeAppleMetal, wantModel: AcceleratorModelM4Ultra},
		{input: "apple-metal-m4-ultra", wantVendor: AcceleratorTypeAppleMetal, wantModel: AcceleratorModelM4Ultra},
		{input: "apple/m4-ultra", wantVendor: AcceleratorTypeAppleMetal, wantModel: AcceleratorModelM4Ultra},
		{input: "apple-m4-ultra", wantVendor: AcceleratorTypeAppleMetal, wantModel: AcceleratorModelM4Ultra},
		{input: "apple-mlx/m3-ultra", wantVendor: AcceleratorTypeAppleMLX, wantModel: AcceleratorModelM3Ultra},
		{input: "apple-m3-ultra", wantVendor: AcceleratorTypeAppleMetal, wantModel: AcceleratorModelM3Ultra},

		// Google TPU
		{input: "google", wantVendor: AcceleratorTypeGoogleTPU, wantModel: ""},
		{input: "google-tpu", wantVendor: AcceleratorTypeGoogleTPU, wantModel: ""},
		{input: "google-tpu/tpu-v6e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv6e},
		{input: "google-tpu-tpu-v6e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv6e},
		{input: "google-tpu-v6e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv6e},
		{input: "google/tpu-v6e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv6e},
		{input: "google/v6e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv6e},
		{input: "google-tpu-v5p", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv5p},
		{input: "google-tpu-v5e", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv5e},
		{input: "google-tpu-v4", wantVendor: AcceleratorTypeGoogleTPU, wantModel: AcceleratorModelTPUv4},

		// AWS Neuron
		{input: "aws", wantVendor: AcceleratorTypeAWSNeuron, wantModel: ""},
		{input: "aws-neuron", wantVendor: AcceleratorTypeAWSNeuron, wantModel: ""},
		{input: "aws-neuron/trn2", wantVendor: AcceleratorTypeAWSNeuron, wantModel: AcceleratorModelTrn2},
		{input: "aws-neuron-trn2", wantVendor: AcceleratorTypeAWSNeuron, wantModel: AcceleratorModelTrn2},
		{input: "aws-trn2", wantVendor: AcceleratorTypeAWSNeuron, wantModel: AcceleratorModelTrn2},
		{input: "aws/trn1", wantVendor: AcceleratorTypeAWSNeuron, wantModel: AcceleratorModelTrn1},
		{input: "aws-neuron/inf2", wantVendor: AcceleratorTypeAWSNeuron, wantModel: AcceleratorModelInf2},

		// Intel Gaudi
		{input: "intel", wantVendor: AcceleratorTypeIntelGaudi, wantModel: ""},
		{input: "intel-gaudi", wantVendor: AcceleratorTypeIntelGaudi, wantModel: ""},
		{input: "intel-gaudi/gaudi3", wantVendor: AcceleratorTypeIntelGaudi, wantModel: AcceleratorModelGaudi3},
		{input: "intel-gaudi-gaudi3", wantVendor: AcceleratorTypeIntelGaudi, wantModel: AcceleratorModelGaudi3},
		{input: "intel-gaudi3", wantVendor: AcceleratorTypeIntelGaudi, wantModel: AcceleratorModelGaudi3},
		{input: "intel/gaudi2", wantVendor: AcceleratorTypeIntelGaudi, wantModel: AcceleratorModelGaudi2},
	}

	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			gotVendor := tc.input.Vendor()
			if gotVendor != tc.wantVendor {
				t.Errorf("Vendor() = %q, want %q", gotVendor, tc.wantVendor)
			}
			gotModel := tc.input.Model()
			if gotModel != tc.wantModel {
				t.Errorf("Model() = %q, want %q", gotModel, tc.wantModel)
			}
		})
	}
}
