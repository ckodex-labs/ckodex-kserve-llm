/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

import (
	servingv1 "github.com/ckodex-labs/kserve-llm-operator/api/v1"
)

func convertRouterToV1(src RouterSpec) servingv1.RouterSpec {
	dst := servingv1.RouterSpec{
		Gateway: servingv1.GatewaySpec{},
	}
	if src.Scheduler != nil {
		dst.Scheduler = convertSchedulerToV1(src.Scheduler)
	}
	if src.Gateway.Managed != nil {
		dst.Gateway.Managed = &servingv1.ManagedGatewaySpec{GatewayClassName: src.Gateway.Managed.GatewayClassName}
	}
	if src.Gateway.ExistingRef != nil {
		dst.Gateway.ExistingRef = &servingv1.GatewayRef{Name: src.Gateway.ExistingRef.Name, Namespace: src.Gateway.ExistingRef.Namespace}
	}
	if src.Route.HTTPRoute != nil {
		dst.Route.HTTPRoute = &servingv1.HTTPRouteSpec{
			Hostnames:  deepCopyStrings(src.Route.HTTPRoute.Hostnames),
			Resilience: convertResilienceToV1(src.Route.HTTPRoute.Resilience),
		}
	}
	return dst
}

func convertRouterFromV1(src servingv1.RouterSpec) RouterSpec {
	dst := RouterSpec{
		Gateway: GatewaySpec{},
	}
	if src.Scheduler != nil {
		dst.Scheduler = convertSchedulerFromV1(src.Scheduler)
	}
	if src.Gateway.Managed != nil {
		dst.Gateway.Managed = &ManagedGatewaySpec{GatewayClassName: src.Gateway.Managed.GatewayClassName}
	}
	if src.Gateway.ExistingRef != nil {
		dst.Gateway.ExistingRef = &GatewayRef{Name: src.Gateway.ExistingRef.Name, Namespace: src.Gateway.ExistingRef.Namespace}
	}
	if src.Route.HTTPRoute != nil {
		dst.Route.HTTPRoute = &HTTPRouteSpec{
			Hostnames:  deepCopyStrings(src.Route.HTTPRoute.Hostnames),
			Resilience: convertResilienceFromV1(src.Route.HTTPRoute.Resilience),
		}
	}
	return dst
}

func convertResilienceToV1(src *ResilienceSpec) *servingv1.ResilienceSpec {
	if src == nil {
		return nil
	}
	return &servingv1.ResilienceSpec{Timeout: src.Timeout, MaxRetries: src.MaxRetries, RetryOn: src.RetryOn}
}

func convertResilienceFromV1(src *servingv1.ResilienceSpec) *ResilienceSpec {
	if src == nil {
		return nil
	}
	return &ResilienceSpec{Timeout: src.Timeout, MaxRetries: src.MaxRetries, RetryOn: src.RetryOn}
}

func convertSchedulerToV1(src *SchedulerSpec) *servingv1.SchedulerSpec {
	dst := &servingv1.SchedulerSpec{
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

func convertSchedulerFromV1(src *servingv1.SchedulerSpec) *SchedulerSpec {
	dst := &SchedulerSpec{
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
