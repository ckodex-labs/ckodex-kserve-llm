package controller

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchedulerNeedsEarlyFence(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		scheduler, gateway, enabled bool
		status                      metav1.ConditionStatus
		observed                    int64
		canary, want                bool
	}{
		{"no scheduler", false, false, false, "", 0, false, false},
		{"gateway disabled", true, false, true, metav1.ConditionTrue, 2, false, true},
		{"scheduler disabled", true, true, false, metav1.ConditionTrue, 2, false, true},
		{"condition absent", true, true, true, "", 0, false, true},
		{"condition false", true, true, true, metav1.ConditionFalse, 2, false, true},
		{"condition unknown", true, true, true, metav1.ConditionUnknown, 2, false, true},
		{"condition stale", true, true, true, metav1.ConditionTrue, 1, false, true},
		{"canary requested", true, true, true, metav1.ConditionTrue, 2, true, true},
		{"current ready", true, true, true, metav1.ConditionTrue, 2, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
			if tc.scheduler {
				service.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
			}
			if tc.canary {
				service.Spec.Canary = &servingv1alpha2.CanarySpec{}
			}
			if tc.status != "" {
				service.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionSchedulerReady, Status: tc.status, ObservedGeneration: tc.observed}}
			}
			if got := schedulerNeedsEarlyFence(service, tc.gateway, tc.enabled); got != tc.want {
				t.Fatalf("schedulerNeedsEarlyFence() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSchedulerBlockedConditionsReplaceReady(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Generation: 2}}
	service.Status.Conditions = []metav1.Condition{
		{Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionTrue, Reason: "EndpointPickerReady"},
		{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Ready"},
	}
	condition := schedulerCondition(service, metav1.ConditionFalse, "SchedulerCanaryUnsupported", "scheduler and canary cannot be combined")
	setSchedulerBlockedConditions(service, condition)
	for _, conditionType := range []string{servingv1alpha2.ConditionSchedulerReady, servingv1alpha2.ConditionReady} {
		got := meta.FindStatusCondition(service.Status.Conditions, conditionType)
		if got == nil || got.Status != metav1.ConditionFalse || got.ObservedGeneration != 2 {
			t.Fatalf("condition %s = %#v, want false for generation 2", conditionType, got)
		}
	}
}

func TestGatewayFeatureDisabledMarksBothConditions(t *testing.T) {
	service := &servingv1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Generation: 3}}
	service.Status.Conditions = []metav1.Condition{
		{Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionTrue, Reason: "EndpointPickerReady"},
		{Type: servingv1alpha2.ConditionReady, Status: metav1.ConditionTrue, Reason: "Ready"},
	}
	condition := schedulerCondition(service, metav1.ConditionFalse, "GatewayFeatureDisabled", "gateway feature is disabled")
	setSchedulerBlockedConditions(service, condition)
	for _, kind := range []string{servingv1alpha2.ConditionSchedulerReady, servingv1alpha2.ConditionReady} {
		got := meta.FindStatusCondition(service.Status.Conditions, kind)
		if got == nil || got.Status != metav1.ConditionFalse || got.Reason != "GatewayFeatureDisabled" || got.ObservedGeneration != 3 {
			t.Fatalf("condition %s = %#v, want gateway-disabled false for generation 3", kind, got)
		}
	}
}
