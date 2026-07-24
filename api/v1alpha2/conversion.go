/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// Hub-and-spoke conversion: v1alpha2 is the spoke, api/v1 is the hub (storage version).
//
// controller-runtime requires ConvertTo and ConvertFrom to be defined in the same package
// as the type. The conversion logic lives here; v1 types are imported without circular risk
// because v1 does not import v1alpha2.

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
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

	if src.Spec.Prefill != nil || src.Spec.Worker != nil || src.Spec.KVCache != nil {
		dst.Spec.Experimental = &servingv1.ExperimentalSpec{}
		if src.Spec.Prefill != nil {
			dst.Spec.Experimental.Prefill = &servingv1.PrefillSpec{
				Replicas: src.Spec.Prefill.Replicas,
				Template: src.Spec.Prefill.Template,
			}
		}
		if src.Spec.Worker != nil {
			dst.Spec.Experimental.Worker = &servingv1.WorkerSpec{
				Template: src.Spec.Worker.Template,
			}
		}
		if src.Spec.KVCache != nil {
			dst.Spec.Experimental.KVCache = convertKVCacheToV1(src.Spec.KVCache)
		}
	}

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.URL = src.Status.URL
	dst.Status.Replicas = src.Status.Replicas
	dst.Status.ModelReady = src.Status.ModelReady
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
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

	if src.Spec.Experimental != nil {
		if src.Spec.Experimental.Prefill != nil {
			dst.Spec.Prefill = &PrefillSpec{
				Replicas: src.Spec.Experimental.Prefill.Replicas,
				Template: src.Spec.Experimental.Prefill.Template,
			}
		}
		if src.Spec.Experimental.Worker != nil {
			dst.Spec.Worker = &WorkerSpec{
				Template: src.Spec.Experimental.Worker.Template,
			}
		}
		if src.Spec.Experimental.KVCache != nil {
			dst.Spec.KVCache = convertKVCacheFromV1(src.Spec.Experimental.KVCache)
		}
	}

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.URL = src.Status.URL
	dst.Status.Replicas = src.Status.Replicas
	dst.Status.ModelReady = src.Status.ModelReady
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration
	return nil
}

func convertKVCacheToV1(src *KVCacheSpec) *servingv1.KVCacheSpec {
	if src == nil {
		return nil
	}
	dst := &servingv1.KVCacheSpec{Dtype: src.Dtype, SwapSpaceGB: src.SwapSpaceGB}
	if src.Transfer != nil {
		dst.Transfer = &servingv1.KVTransferSpec{Connector: src.Transfer.Connector, Role: src.Transfer.Role, ExtraConfig: src.Transfer.ExtraConfig, Env: deepCopyEnv(src.Transfer.Env)}
	}
	return dst
}

func convertKVCacheFromV1(src *servingv1.KVCacheSpec) *KVCacheSpec {
	if src == nil {
		return nil
	}
	dst := &KVCacheSpec{Dtype: src.Dtype, SwapSpaceGB: src.SwapSpaceGB}
	if src.Transfer != nil {
		dst.Transfer = &KVTransferSpec{Connector: src.Transfer.Connector, Role: src.Transfer.Role, ExtraConfig: src.Transfer.ExtraConfig, Env: deepCopyEnv(src.Transfer.Env)}
	}
	return dst
}

func deepCopyEnv(src []corev1.EnvVar) []corev1.EnvVar {
	if src == nil {
		return nil
	}
	dst := make([]corev1.EnvVar, len(src))
	for i := range src {
		src[i].DeepCopyInto(&dst[i])
	}
	return dst
}

// ----- helpers -----

func convertModelSpecToV1(src ModelSpec) servingv1.ModelSpec {
	dst := servingv1.ModelSpec{URI: src.URI, Name: src.Name}
	if src.Storage != nil {
		dst.Storage = &servingv1.StorageSpec{
			ServiceAccountName:  src.Storage.ServiceAccountName,
			StorageContainerRef: src.Storage.StorageContainerRef,
			VaultRef:            src.Storage.VaultRef,
			VaultAddr:           src.Storage.VaultAddr,
		}
		if src.Storage.SecretRef != nil {
			dst.Storage.SecretRef = src.Storage.SecretRef.DeepCopy()
		}
	}
	return dst
}

func convertModelSpecFromV1(src servingv1.ModelSpec) ModelSpec {
	dst := ModelSpec{URI: src.URI, Name: src.Name}
	if src.Storage != nil {
		dst.Storage = &StorageSpec{
			ServiceAccountName:  src.Storage.ServiceAccountName,
			StorageContainerRef: src.Storage.StorageContainerRef,
			VaultRef:            src.Storage.VaultRef,
			VaultAddr:           src.Storage.VaultAddr,
		}
		if src.Storage.SecretRef != nil {
			dst.Storage.SecretRef = src.Storage.SecretRef.DeepCopy()
		}
	}
	return dst
}

func convertParallelismToV1(src *ParallelismSpec) *servingv1.ParallelismSpec {
	if src == nil {
		return nil
	}
	return &servingv1.ParallelismSpec{Tensor: src.Tensor, Data: src.Data, DataLocal: src.DataLocal, Expert: src.Expert}
}

func convertParallelismFromV1(src *servingv1.ParallelismSpec) *ParallelismSpec {
	if src == nil {
		return nil
	}
	return &ParallelismSpec{Tensor: src.Tensor, Data: src.Data, DataLocal: src.DataLocal, Expert: src.Expert}
}

func convertScalingToV1(src *ScalingSpec) *servingv1.ScalingSpec {
	if src == nil {
		return nil
	}
	dst := &servingv1.ScalingSpec{MinReplicas: src.MinReplicas, MaxReplicas: src.MaxReplicas}
	if src.WVA != nil {
		dst.WVA = &servingv1.WVASpec{VariantCost: src.WVA.VariantCost}
	}
	if src.KEDA != nil {
		dst.KEDA = &servingv1.KEDASpec{
			PollingInterval:       src.KEDA.PollingInterval,
			CooldownPeriod:        src.KEDA.CooldownPeriod,
			InitialCooldownPeriod: src.KEDA.InitialCooldownPeriod,
			IdleReplicaCount:      src.KEDA.IdleReplicaCount,
		}
		if src.KEDA.Fallback != nil {
			dst.KEDA.Fallback = &servingv1.KEDAFallbackSpec{
				FailureThreshold: src.KEDA.Fallback.FailureThreshold,
				Replicas:         src.KEDA.Fallback.Replicas,
			}
		}
	}
	if src.HPA != nil {
		dst.HPA = &servingv1.HPASpec{TargetCPUUtilizationPercentage: src.HPA.TargetCPUUtilizationPercentage}
	}
	return dst
}

func convertScalingFromV1(src *servingv1.ScalingSpec) *ScalingSpec {
	if src == nil {
		return nil
	}
	dst := &ScalingSpec{MinReplicas: src.MinReplicas, MaxReplicas: src.MaxReplicas}
	if src.WVA != nil {
		dst.WVA = &WVASpec{VariantCost: src.WVA.VariantCost}
	}
	if src.KEDA != nil {
		dst.KEDA = &KEDASpec{
			PollingInterval:       src.KEDA.PollingInterval,
			CooldownPeriod:        src.KEDA.CooldownPeriod,
			InitialCooldownPeriod: src.KEDA.InitialCooldownPeriod,
			IdleReplicaCount:      src.KEDA.IdleReplicaCount,
		}
		if src.KEDA.Fallback != nil {
			dst.KEDA.Fallback = &KEDAFallbackSpec{
				FailureThreshold: src.KEDA.Fallback.FailureThreshold,
				Replicas:         src.KEDA.Fallback.Replicas,
			}
		}
	}
	if src.HPA != nil {
		dst.HPA = &HPASpec{TargetCPUUtilizationPercentage: src.HPA.TargetCPUUtilizationPercentage}
	}
	return dst
}

func convertRouterToV1(src RouterSpec) servingv1.RouterSpec {
	dst := servingv1.RouterSpec{
		Gateway:   servingv1.GatewaySpec{},
		Scheduler: convertSchedulerToV1(src.Scheduler),
	}
	if src.Gateway.Managed != nil {
		dst.Gateway.Managed = &servingv1.ManagedGatewaySpec{GatewayClassName: src.Gateway.Managed.GatewayClassName}
	}
	if src.Gateway.ExistingRef != nil {
		dst.Gateway.ExistingRef = &servingv1.GatewayRef{Name: src.Gateway.ExistingRef.Name, Namespace: src.Gateway.ExistingRef.Namespace}
	}
	if src.Route.HTTPRoute != nil {
		dst.Route.HTTPRoute = &servingv1.HTTPRouteSpec{Hostnames: src.Route.HTTPRoute.Hostnames}
	}
	return dst
}

func convertRouterFromV1(src servingv1.RouterSpec) RouterSpec {
	dst := RouterSpec{
		Gateway:   GatewaySpec{},
		Scheduler: convertSchedulerFromV1(src.Scheduler),
	}
	if src.Gateway.Managed != nil {
		dst.Gateway.Managed = &ManagedGatewaySpec{GatewayClassName: src.Gateway.Managed.GatewayClassName}
	}
	if src.Gateway.ExistingRef != nil {
		dst.Gateway.ExistingRef = &GatewayRef{Name: src.Gateway.ExistingRef.Name, Namespace: src.Gateway.ExistingRef.Namespace}
	}
	if src.Route.HTTPRoute != nil {
		dst.Route.HTTPRoute = &HTTPRouteSpec{Hostnames: src.Route.HTTPRoute.Hostnames}
	}
	return dst
}

func convertSchedulerToV1(src SchedulerSpec) servingv1.SchedulerSpec {
	dst := servingv1.SchedulerSpec{
		Replicas: src.Replicas,
		Pool:     servingv1.InferencePoolSpec{Selector: src.Pool.Selector},
	}
	if src.Config != nil && src.Config.Ref != nil {
		dst.Config = &servingv1.SchedulerConfigSpec{
			Ref: &servingv1.SchedulerConfigRef{Name: src.Config.Ref.Name, Key: src.Config.Ref.Key},
		}
	}
	return dst
}

func convertSchedulerFromV1(src servingv1.SchedulerSpec) SchedulerSpec {
	dst := SchedulerSpec{
		Replicas: src.Replicas,
		Pool:     InferencePoolSpec{Selector: src.Pool.Selector},
	}
	if src.Config != nil && src.Config.Ref != nil {
		dst.Config = &SchedulerConfigSpec{
			Ref: &SchedulerConfigRef{Name: src.Config.Ref.Name, Key: src.Config.Ref.Key},
		}
	}
	return dst
}

func convertBaseRefsToV1(src []ConfigReference) []servingv1.ConfigReference {
	if src == nil {
		return nil
	}
	dst := make([]servingv1.ConfigReference, len(src))
	for i, r := range src {
		dst[i] = servingv1.ConfigReference{Name: r.Name}
	}
	return dst
}

func convertBaseRefsFromV1(src []servingv1.ConfigReference) []ConfigReference {
	if src == nil {
		return nil
	}
	dst := make([]ConfigReference, len(src))
	for i, r := range src {
		dst[i] = ConfigReference{Name: r.Name}
	}
	return dst
}

func convertSLOToV1(src *SLOSpec) *servingv1.SLOSpec {
	if src == nil {
		return nil
	}
	return &servingv1.SLOSpec{
		TargetP99LatencyMs: src.TargetP99LatencyMs,
		TargetAvailability: src.TargetAvailability,
		ErrorBudgetDays:    src.ErrorBudgetDays,
	}
}

func convertSLOFromV1(src *servingv1.SLOSpec) *SLOSpec {
	if src == nil {
		return nil
	}
	return &SLOSpec{
		TargetP99LatencyMs: src.TargetP99LatencyMs,
		TargetAvailability: src.TargetAvailability,
		ErrorBudgetDays:    src.ErrorBudgetDays,
	}
}

func convertCanaryToV1(src *CanarySpec) *servingv1.CanarySpec {
	if src == nil {
		return nil
	}
	return &servingv1.CanarySpec{Weight: src.Weight, BaseModel: src.BaseModel}
}

func convertCanaryFromV1(src *servingv1.CanarySpec) *CanarySpec {
	if src == nil {
		return nil
	}
	return &CanarySpec{Weight: src.Weight, BaseModel: src.BaseModel}
}
