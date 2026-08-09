/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package scheduler

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	eppNameSuffix             = "-epp"
	schedulerConfigNameSuffix = "-scheduler-config"
)

// InferencePoolGVK identifies the GA Gateway API Inference Extension pool.
var InferencePoolGVK = schema.GroupVersionKind{
	Group: "inference.networking.k8s.io", Version: "v1", Kind: "InferencePool",
}

func eppName(serviceName string) string {
	return serviceName + eppNameSuffix
}

func schedulerConfigName(serviceName string) string {
	return serviceName + schedulerConfigNameSuffix
}
