/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package inference

// --- Zero-Copy Network Config ---

// ZeroCopyConfig defines infrastructure settings required for GPU-Direct RDMA.
// When enabled, the operator provisions pods with SR-IOV network definitions
// allowing NCCL to bypass the CPU and host memory completely during tensor parallel sync.
type ZeroCopyConfig struct {
	// EnableRDMA injects Mellanox/Nvidia SR-IOV resources into the worker pods.
	EnableRDMA bool `json:"enableRDMA"`

	// ResourceName is the k8s extended resource for the RDMA VF (e.g., mellanox.com/cx5_sriov).
	ResourceName string `json:"resourceName"`

	// SharedMemorySize is the size of the /dev/shm mount for collective communication staging.
	SharedMemorySize string `json:"sharedMemorySize"` // e.g., "16Gi"
}

// ApplyZeroCopy applies RDMA/SR-IOV multi-net annotations to a pod template.
func (z *ZeroCopyConfig) ApplyZeroCopy(annotations, limits map[string]string) {
	if !z.EnableRDMA {
		return
	}

	// Multus CNI annotation for the secondary SR-IOV interface (roce-network)
	if annotations != nil {
		annotations["k8s.v1.cni.cncf.io/networks"] = "roce-network"
	}

	// Request the hardware Virtual Function (VF)
	if limits != nil && z.ResourceName != "" {
		limits[z.ResourceName] = "1"
	}
}
