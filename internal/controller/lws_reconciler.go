/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package controller implements LeaderWorkerSet integration for distributed multi-node inference.
package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)


// Reconciler manages LeaderWorkerSet resources for distributed inference.
// Maps ParallelismSpec → multi-node GPU topology.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates/updates a LeaderWorkerSet for multi-node inference.
func (r *Reconciler) Reconcile(ctx context.Context, llmSvc *servingv1alpha2.LLMInferenceService) error {
	if llmSvc.Spec.Parallelism == nil {
		return nil // No parallelism, no LWS needed
	}

	logger := log.FromContext(ctx).WithValues("component", "lws")

	// Calculate total workers from parallelism spec
	totalWorkers := r.calculateWorkers(llmSvc.Spec.Parallelism)
	if totalWorkers <= 1 {
		return nil // Single node, no LWS needed
	}

	name := llmSvc.Name + "-lws"

	// Build LeaderWorkerSet as unstructured (CRD may not be vendored)
	desired := r.buildLWS(llmSvc, name, totalWorkers)

	if err := controllerutil.SetControllerReference(llmSvc, desired, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	var existing unstructured.Unstructured
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: llmSvc.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("creating LeaderWorkerSet", "name", name, "workers", totalWorkers)
			return r.Create(ctx, desired)
		}
		return err
	}

	// Update spec
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// calculateWorkers determines the number of worker nodes from parallelism.
func (r *Reconciler) calculateWorkers(p *servingv1alpha2.ParallelismSpec) int32 {
	if p == nil {
		return 1
	}
	workers := int32(1)

	// Tensor parallelism: split model across GPUs/nodes
	if p.Tensor != nil && *p.Tensor > 1 {
		workers *= *p.Tensor
	}

	// Data parallelism: replicate across nodes
	if p.Data != nil && *p.Data > 1 {
		workers *= *p.Data
	}

	// Expert parallelism (MoE): double workers when enabled
	if p.Expert {
		workers *= 2
	}

	return workers
}

// buildLWS constructs the LeaderWorkerSet spec as unstructured.
func (r *Reconciler) buildLWS(llmSvc *servingv1alpha2.LLMInferenceService, name string, totalWorkers int32) *unstructured.Unstructured {
	labels := map[string]interface{}{
		"app.kubernetes.io/name":       "llminferenceservice",
		"app.kubernetes.io/instance":   llmSvc.Name,
		"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
		"serving.ckodex.com/model":     llmSvc.Spec.Model.Name,
		"serving.ckodex.com/role":      "distributed-inference",
	}

	// Worker count: total - 1 leader
	workerCount := totalWorkers - 1
	if workerCount < 1 {
		workerCount = 1
	}

	// Build vLLM args based on parallelism
	vllmArgs := r.buildVLLMArgs(llmSvc)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "leaderworkerset.x-k8s.io/v1",
			"kind":       "LeaderWorkerSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": llmSvc.Namespace,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"leaderWorkerTemplate": map[string]interface{}{
					"size": int64(workerCount + 1), // leader + workers
					"leaderTemplate": map[string]interface{}{
						"metadata": map[string]interface{}{"labels": labels},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "vllm-leader",
									"image": "vllm/vllm-openai:latest",
									"args":  vllmArgs,
									"ports": []interface{}{
										map[string]interface{}{"containerPort": int64(8000), "name": "http"},
										map[string]interface{}{"containerPort": int64(8001), "name": "grpc"},
									},
									"env": []interface{}{
										map[string]interface{}{"name": "ROLE", "value": "leader"},
									},
								},
							},
						},
					},
					"workerTemplate": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "vllm-worker",
									"image": "vllm/vllm-openai:latest",
									"args":  vllmArgs,
									"env": []interface{}{
										map[string]interface{}{"name": "ROLE", "value": "worker"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return obj
}

// buildVLLMArgs generates vLLM command-line arguments from parallelism spec.
func (r *Reconciler) buildVLLMArgs(llmSvc *servingv1alpha2.LLMInferenceService) []interface{} {
	args := []interface{}{
		"--model", llmSvc.Spec.Model.URI,
		"--served-model-name", llmSvc.Spec.Model.Name,
	}

	if p := llmSvc.Spec.Parallelism; p != nil {
		if p.Tensor != nil && *p.Tensor > 1 {
			args = append(args, "--tensor-parallel-size", fmt.Sprintf("%d", *p.Tensor))
		}
		if p.Data != nil && *p.Data > 1 {
			args = append(args, "--data-parallel-size", fmt.Sprintf("%d", *p.Data))
		}

		// Disaggregated prefill-decode mode (indicated by Prefill spec presence)
		if llmSvc.Spec.Prefill != nil {
			args = append(args, "--enable-disaggregated-prefill")
		}
	}

	return args
}

// PrefillDecodeConfig returns vLLM args for disaggregated prefill-decode.
type PrefillDecodeConfig struct {
	// PrefillWorkers is the number of workers dedicated to prefill.
	PrefillWorkers int32
	// DecodeWorkers is the number of workers dedicated to decode.
	DecodeWorkers int32
	// KVTransferMethod is the method for transferring KV cache.
	KVTransferMethod string // "nccl" or "rdma"
}

// DefaultPrefillDecodeConfig returns defaults.
func DefaultPrefillDecodeConfig() PrefillDecodeConfig {
	return PrefillDecodeConfig{
		PrefillWorkers:   1,
		DecodeWorkers:    1,
		KVTransferMethod: "nccl",
	}
}

// GPUTopology maps parallelism spec to GPU requirements.
type GPUTopology struct {
	TotalGPUs     int32
	GPUsPerNode   int32
	NodesRequired int32
	Placement     string // "compact" or "spread"
}

// ComputeGPUTopology calculates GPU topology requirements.
func ComputeGPUTopology(p *servingv1alpha2.ParallelismSpec) GPUTopology {
	if p == nil {
		return GPUTopology{TotalGPUs: 1, GPUsPerNode: 1, NodesRequired: 1, Placement: "compact"}
	}
	tp := int32(1)
	dp := int32(1)
	ep := int32(1)

	if p.Tensor != nil {
		tp = *p.Tensor
	}
	if p.Data != nil {
		dp = *p.Data
	}
	if p.Expert {
		ep = 2 // Expert parallelism doubles GPU requirement
	}

	totalGPUs := tp * dp * ep
	gpusPerNode := tp // Tensor parallelism within a node
	nodesRequired := totalGPUs / gpusPerNode
	if nodesRequired < 1 {
		nodesRequired = 1
	}

	placement := "compact"
	if p.DataLocal != nil && *p.DataLocal > 1 {
		placement = "spread"
	}

	return GPUTopology{
		TotalGPUs:     totalGPUs,
		GPUsPerNode:   gpusPerNode,
		NodesRequired: nodesRequired,
		Placement:     placement,
	}
}

func init() {
	_ = metav1.Now // keep import
}
