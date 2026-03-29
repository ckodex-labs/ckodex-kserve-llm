/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package v1 contains the stable v1 API types for the CKodex KServe LLM Operator.
//
// v1 is the storage version. v1alpha2 converts to/from v1 via the conversion webhook.
//
// Key differences from v1alpha2:
//   - spec.prefill and spec.worker moved to spec.experimental.prefill / spec.experimental.worker
//   - All other fields are identical
//
// +kubebuilder:object:generate=true
// +groupName=serving.ckodex.com
package v1

const (
	// GroupName is the group name for the CKodex serving API.
	GroupName = "serving.ckodex.com"

	// GroupVersion is the version for the CKodex serving API.
	GroupVersion = "v1"
)
