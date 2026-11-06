package gateway

import (
	"testing"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchedulerRouteTransitions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		scheduler  bool
		condition  *metav1.Condition
		wantTarget string
	}{
		{"not requested", false, nil, "Service"},
		{"not yet observed", true, nil, "blocked"},
		{"unavailable", true, &metav1.Condition{Status: metav1.ConditionFalse, ObservedGeneration: 2}, "blocked"},
		{"unknown", true, &metav1.Condition{Status: metav1.ConditionUnknown, ObservedGeneration: 2}, "blocked"},
		{"stale ready", true, &metav1.Condition{Status: metav1.ConditionTrue, ObservedGeneration: 1}, "blocked"},
		{"current ready", true, &metav1.Condition{Status: metav1.ConditionTrue, ObservedGeneration: 2}, "InferencePool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := baseLLMSvc("model")
			svc.Generation = 2
			if tc.scheduler {
				svc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
			}
			if tc.condition != nil {
				cond := *tc.condition
				cond.Type = servingv1alpha2.ConditionSchedulerReady
				svc.Status.Conditions = []metav1.Condition{cond}
			}
			http := BuildHTTPRoute(svc, nil)
			grpc := BuildGRPCRoute(svc)
			if len(http.Spec.Rules) == 0 || len(grpc.Spec.Rules) == 0 {
				t.Fatal("route has no matching rules")
			}
			for _, rule := range http.Spec.Rules {
				if tc.wantTarget == "blocked" {
					if len(rule.BackendRefs) != 0 || len(rule.Filters) != 0 {
						t.Fatal("blocked HTTP rule can forward or filter")
					}
				} else if len(rule.BackendRefs) != 1 {
					t.Fatal("forwarding HTTP rule must have exactly one backend")
				} else if tc.wantTarget == "InferencePool" && (rule.BackendRefs[0].Kind == nil || string(*rule.BackendRefs[0].Kind) != "InferencePool") {
					t.Fatal("ready scheduler did not route to InferencePool")
				} else if tc.wantTarget == "Service" && rule.BackendRefs[0].Kind != nil {
					t.Fatal("unconfigured scheduler did not route to Service")
				}
			}
			for _, rule := range grpc.Spec.Rules {
				if tc.scheduler && (len(rule.BackendRefs) != 0 || len(rule.Filters) != 0) {
					t.Fatal("gRPC bypasses a requested scheduler")
				}
				if !tc.scheduler && len(rule.BackendRefs) != 1 {
					t.Fatal("ordinary gRPC route lost its service backend")
				}
			}
		})
	}
}

func TestCanaryNeverBypassesRequestedScheduler(t *testing.T) {
	svc := canarySvc("model", "default", "stable", 25)
	svc.Spec.Router.Scheduler = &servingv1alpha2.SchedulerSpec{}
	svc.Generation = 1
	svc.Status.Conditions = []metav1.Condition{{Type: servingv1alpha2.ConditionSchedulerReady, Status: metav1.ConditionTrue, ObservedGeneration: 1}}
	for _, rule := range BuildCanaryHTTPRoute(svc, nil).Spec.Rules {
		if len(rule.BackendRefs) != 0 || len(rule.Filters) != 0 {
			t.Fatal("invalid scheduler and canary combination forwarded traffic")
		}
	}
}
