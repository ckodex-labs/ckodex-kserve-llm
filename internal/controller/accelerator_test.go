/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func TestApplyAcceleratorResources(t *testing.T) {
	tests := []struct {
		name         string
		spec         *servingv1alpha2.AcceleratorSpec
		wantResource corev1.ResourceName
		wantCount    int64
	}{
		{
			name:         "NVIDIA direct",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: servingv1alpha2.AcceleratorTypeNVIDIA, Count: ptr.To(int32(2))},
			wantResource: "nvidia.com/gpu",
			wantCount:    2,
		},
		{
			name:         "NVIDIA compound",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: "nvidia/gb300", Count: ptr.To(int32(1))},
			wantResource: "nvidia.com/gpu",
			wantCount:    1,
		},
		{
			name:         "ROCm MI300X",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: "rocm/mi300x", Count: ptr.To(int32(4))},
			wantResource: "amd.com/gpu",
			wantCount:    4,
		},
		{
			name:         "Google TPU v6e",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: "google-tpu/tpu-v6e", Count: ptr.To(int32(8))},
			wantResource: "google.com/tpu",
			wantCount:    8,
		},
		{
			name:         "AWS Neuron Trainium2",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: "aws-neuron/trn2", Count: ptr.To(int32(4))},
			wantResource: "aws.amazon.com/neuron",
			wantCount:    4,
		},
		{
			name:         "Intel Gaudi3",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: "intel-gaudi/gaudi3", Count: ptr.To(int32(4))},
			wantResource: "habana.ai/gaudi",
			wantCount:    4,
		},
		{
			name:         "Nil spec does nothing",
			spec:         nil,
			wantResource: "",
			wantCount:    0,
		},
		{
			name:         "CPU does not inject device resource",
			spec:         &servingv1alpha2.AcceleratorSpec{Type: servingv1alpha2.AcceleratorTypeCPU},
			wantResource: "",
			wantCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := &corev1.Container{}
			applyAcceleratorResources(container, tc.spec)
			if tc.wantResource == "" {
				if len(container.Resources.Requests) != 0 || len(container.Resources.Limits) != 0 {
					t.Errorf("expected no resources, got requests: %v, limits: %v", container.Resources.Requests, container.Resources.Limits)
				}
				return
			}
			reqQty := container.Resources.Requests[tc.wantResource]
			limQty := container.Resources.Limits[tc.wantResource]
			if reqQty.Value() != tc.wantCount {
				t.Errorf("request %s = %d, want %d", tc.wantResource, reqQty.Value(), tc.wantCount)
			}
			if limQty.Value() != tc.wantCount {
				t.Errorf("limit %s = %d, want %d", tc.wantResource, limQty.Value(), tc.wantCount)
			}
		})
	}
}
