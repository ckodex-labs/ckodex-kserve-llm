/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// Hub-and-spoke conversion: v1alpha2 is the spoke, api/v1 is the hub (storage version).
//
// controller-runtime requires ConvertTo and ConvertFrom to be defined in the same package
// as the type. The conversion logic lives here; v1 types are imported without circular risk
// because v1 does not import v1alpha2. The beta CRD profile binds this spoke to the
// controller-runtime conversion webhook at /convert.

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts v1alpha2.LLMInferenceService → v1.LLMInferenceService (spoke → hub).
func (src *LLMInferenceService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*servingv1.LLMInferenceService)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Model = convertModelSpecToV1(src.Spec.Model)
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Parallelism = convertParallelismToV1(src.Spec.Parallelism)
	dst.Spec.Scaling = convertScalingToV1(src.Spec.Scaling)
	dst.Spec.Template = src.Spec.Template
	dst.Spec.Router = convertRouterToV1(src.Spec.Router)
	dst.Spec.BaseRefs = convertBaseRefsToV1(src.Spec.BaseRefs)
	dst.Spec.AutoOptimize = src.Spec.AutoOptimize
	dst.Spec.AllowedTenants = src.Spec.AllowedTenants
	dst.Spec.CostAllocationTags = src.Spec.CostAllocationTags
	dst.Spec.SLO = convertSLOToV1(src.Spec.SLO)
	dst.Spec.Canary = convertCanaryToV1(src.Spec.Canary)

	convertExperimentalToV1(src, dst)

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.URL = src.Status.URL
	dst.Status.Replicas = src.Status.Replicas
	dst.Status.ModelReady = src.Status.ModelReady
	dst.Status.ModelRevision = src.Status.ModelRevision
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	dst.Status.StatePlanes = convertStatePlanesToV1(src.Status.StatePlanes)
	dst.Status.Optimized = src.Status.Optimized
	dst.Status.DetectedHardware = src.Status.DetectedHardware
	dst.Status.AdaptiveMetrics = convertAdaptiveMetricsToV1(src.Status.AdaptiveMetrics)
	return nil
}

// ConvertFrom converts v1.LLMInferenceService → v1alpha2.LLMInferenceService (hub → spoke).
func (dst *LLMInferenceService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*servingv1.LLMInferenceService)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Model = convertModelSpecFromV1(src.Spec.Model)
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Parallelism = convertParallelismFromV1(src.Spec.Parallelism)
	dst.Spec.Scaling = convertScalingFromV1(src.Spec.Scaling)
	dst.Spec.Template = src.Spec.Template
	dst.Spec.Router = convertRouterFromV1(src.Spec.Router)
	dst.Spec.BaseRefs = convertBaseRefsFromV1(src.Spec.BaseRefs)
	dst.Spec.AutoOptimize = src.Spec.AutoOptimize
	dst.Spec.AllowedTenants = src.Spec.AllowedTenants
	dst.Spec.CostAllocationTags = src.Spec.CostAllocationTags
	dst.Spec.SLO = convertSLOFromV1(src.Spec.SLO)
	dst.Spec.Canary = convertCanaryFromV1(src.Spec.Canary)

	convertExperimentalFromV1(src, dst)

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.URL = src.Status.URL
	dst.Status.Replicas = src.Status.Replicas
	dst.Status.ModelReady = src.Status.ModelReady
	dst.Status.ModelRevision = src.Status.ModelRevision
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	dst.Status.StatePlanes = convertStatePlanesFromV1(src.Status.StatePlanes)
	dst.Status.Optimized = src.Status.Optimized
	dst.Status.DetectedHardware = src.Status.DetectedHardware
	dst.Status.AdaptiveMetrics = convertAdaptiveMetricsFromV1(src.Status.AdaptiveMetrics)
	return nil
}
