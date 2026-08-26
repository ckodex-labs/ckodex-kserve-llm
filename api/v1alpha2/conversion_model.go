/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
)

func convertModelSpecToV1(src ModelSpec) servingv1.ModelSpec {
	dst := servingv1.ModelSpec{URI: src.URI, Name: src.Name, Revision: src.Revision, HardwareAware: src.HardwareAware}
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
		if src.Storage.ExternalSecret != nil {
			dst.Storage.ExternalSecret = convertExternalSecretToV1(src.Storage.ExternalSecret)
		}
	}
	return dst
}

func convertModelSpecFromV1(src servingv1.ModelSpec) ModelSpec {
	dst := ModelSpec{URI: src.URI, Name: src.Name, Revision: src.Revision, HardwareAware: src.HardwareAware}
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
		if src.Storage.ExternalSecret != nil {
			dst.Storage.ExternalSecret = convertExternalSecretFromV1(src.Storage.ExternalSecret)
		}
	}
	return dst
}

func convertExternalSecretToV1(src *ExternalSecretSpec) *servingv1.ExternalSecretSpec {
	if src == nil {
		return nil
	}
	dst := &servingv1.ExternalSecretSpec{
		SecretStoreRef:  servingv1.SecretStoreRef{Name: src.SecretStoreRef.Name, Kind: src.SecretStoreRef.Kind},
		RefreshInterval: src.RefreshInterval,
	}
	if src.Data != nil {
		dst.Data = make([]servingv1.ExternalSecretData, len(src.Data))
		for i, item := range src.Data {
			dst.Data[i] = servingv1.ExternalSecretData{
				SecretKey: item.SecretKey,
				RemoteRef: servingv1.ExternalSecretRemoteRef{Key: item.RemoteRef.Key, Property: item.RemoteRef.Property},
			}
		}
	}
	return dst
}

func convertExternalSecretFromV1(src *servingv1.ExternalSecretSpec) *ExternalSecretSpec {
	if src == nil {
		return nil
	}
	dst := &ExternalSecretSpec{
		SecretStoreRef:  SecretStoreRef{Name: src.SecretStoreRef.Name, Kind: src.SecretStoreRef.Kind},
		RefreshInterval: src.RefreshInterval,
	}
	if src.Data != nil {
		dst.Data = make([]ExternalSecretData, len(src.Data))
		for i, item := range src.Data {
			dst.Data[i] = ExternalSecretData{
				SecretKey: item.SecretKey,
				RemoteRef: ExternalSecretRemoteRef{Key: item.RemoteRef.Key, Property: item.RemoteRef.Property},
			}
		}
	}
	return dst
}

func convertParallelismToV1(src *ParallelismSpec) *servingv1.ParallelismSpec {
	if src == nil {
		return nil
	}
	return &servingv1.ParallelismSpec{Tensor: src.Tensor, Data: src.Data, DataLocal: src.DataLocal, Expert: src.Expert, Pipeline: src.Pipeline, EPLBEnabled: src.EPLBEnabled}
}

func convertParallelismFromV1(src *servingv1.ParallelismSpec) *ParallelismSpec {
	if src == nil {
		return nil
	}
	return &ParallelismSpec{Tensor: src.Tensor, Data: src.Data, DataLocal: src.DataLocal, Expert: src.Expert, Pipeline: src.Pipeline, EPLBEnabled: src.EPLBEnabled}
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
