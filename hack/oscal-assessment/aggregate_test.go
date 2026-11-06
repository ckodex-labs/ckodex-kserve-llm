package main

import "testing"

func TestEvaluationRequiresEveryServiceToPass(t *testing.T) {
	passing := object{Metadata: metadata{Name: "first", Namespace: "inference"}}
	passing.Status.Conditions = []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}{
		{Type: "Compliance-AU-2", Status: "True", Reason: "AuditPersistent"},
	}
	failing := object{Metadata: metadata{Name: "second", Namespace: "inference"}}

	for _, tc := range []struct {
		name     string
		services []object
		want     bool
	}{
		{name: "all services pass", services: []object{passing, passing}, want: true},
		{name: "one service fails", services: []object{passing, failing}, want: false},
		{name: "first service fails", services: []object{failing, passing}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluate(input{Services: tc.services}, "au-2")
			if len(got) != 1 || got[0].Passed != tc.want {
				t.Fatalf("evaluate = %+v, want passed=%t", got, tc.want)
			}
		})
	}
}
