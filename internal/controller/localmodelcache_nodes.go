/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

func (r *LocalModelCacheReconciler) resolveTargetNodes(ctx context.Context, lmc *servingv1alpha2.LocalModelCache) ([]string, error) {
	reader := r.cacheReader()
	seen := map[string]bool{}
	var nodes []string
	if err := r.appendSelectorNodes(ctx, reader, lmc, seen, &nodes); err != nil {
		return nil, err
	}
	if err := r.appendWarmNodes(ctx, reader, lmc, seen, &nodes); err != nil {
		return nil, err
	}
	if lmc.Spec.NodeGroup == nil && len(lmc.Spec.WarmNodes) == 0 {
		if err := appendSchedulableNodes(ctx, reader, seen, &nodes); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (r *LocalModelCacheReconciler) cacheReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func (r *LocalModelCacheReconciler) appendSelectorNodes(ctx context.Context, reader client.Reader, lmc *servingv1alpha2.LocalModelCache, seen map[string]bool, nodes *[]string) error {
	if lmc.Spec.NodeGroup == nil || lmc.Spec.NodeGroup.LabelSelector == nil {
		return nil
	}
	nodeList := &corev1.NodeList{}
	selector, err := metav1.LabelSelectorAsSelector(lmc.Spec.NodeGroup.LabelSelector)
	if err != nil {
		return fmt.Errorf("parse node group selector: %w", err)
	}
	if err := reader.List(ctx, nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return fmt.Errorf("listing nodes by selector: %w", err)
	}
	for _, node := range nodeList.Items {
		appendUniqueNode(seen, nodes, node.Name)
	}
	return nil
}

func (r *LocalModelCacheReconciler) appendWarmNodes(ctx context.Context, reader client.Reader, lmc *servingv1alpha2.LocalModelCache, seen map[string]bool, nodes *[]string) error {
	for _, name := range lmc.Spec.WarmNodes {
		if seen[name] {
			continue
		}
		node := &corev1.Node{}
		if err := reader.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
			if errors.IsNotFound(err) {
				log.FromContext(ctx).Info("WarmNode not found, skipping", "node", name)
				continue
			}
			return fmt.Errorf("checking warm node %s: %w", name, err)
		}
		appendUniqueNode(seen, nodes, name)
	}
	return nil
}

func appendSchedulableNodes(ctx context.Context, reader client.Reader, seen map[string]bool, nodes *[]string) error {
	nodeList := &corev1.NodeList{}
	if err := reader.List(ctx, nodeList); err != nil {
		return fmt.Errorf("listing all nodes: %w", err)
	}
	for _, node := range nodeList.Items {
		if !node.Spec.Unschedulable {
			appendUniqueNode(seen, nodes, node.Name)
		}
	}
	return nil
}

func appendUniqueNode(seen map[string]bool, nodes *[]string, name string) {
	if !seen[name] {
		seen[name] = true
		*nodes = append(*nodes, name)
	}
}
