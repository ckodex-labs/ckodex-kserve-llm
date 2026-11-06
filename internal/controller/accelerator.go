package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func applyAcceleratorResources(container *corev1.Container, accelerator *servingv1alpha2.AcceleratorSpec) {
	if accelerator == nil {
		return
	}
	vendor := accelerator.Vendor()
	var resourceName corev1.ResourceName
	switch vendor {
	case servingv1alpha2.AcceleratorTypeNVIDIA:
		resourceName = "nvidia.com/gpu"
	case servingv1alpha2.AcceleratorTypeROCm:
		resourceName = "amd.com/gpu"
	case servingv1alpha2.AcceleratorTypeGoogleTPU:
		resourceName = "google.com/tpu"
	case servingv1alpha2.AcceleratorTypeAWSNeuron:
		resourceName = "aws.amazon.com/neuron"
	case servingv1alpha2.AcceleratorTypeIntelGaudi:
		resourceName = "habana.ai/gaudi"
	default:
		return
	}

	count := int64(1)
	if accelerator.Count != nil {
		count = int64(*accelerator.Count)
	}
	quantity := *resource.NewQuantity(count, resource.DecimalSI)
	if container.Resources.Requests == nil {
		container.Resources.Requests = corev1.ResourceList{}
	}
	if container.Resources.Limits == nil {
		container.Resources.Limits = corev1.ResourceList{}
	}
	container.Resources.Requests[resourceName] = quantity
	container.Resources.Limits[resourceName] = quantity
}
