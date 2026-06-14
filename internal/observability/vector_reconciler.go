/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	servingv1alpha2 "github.com/ckodex-labs/kserve-llm-operator/api/v1alpha2"
)

// ReconcileVectorConfigMap creates or updates the Vector ConfigMap for an LLMInferenceService.
// otelEndpoint is the operator-level OTEL_EXPORTER_OTLP_ENDPOINT; the service spec can override it.
func ReconcileVectorConfigMap(ctx context.Context, c client.Client, scheme *runtime.Scheme, llmSvc *servingv1alpha2.LLMInferenceService, otelEndpoint string) error {
	sinkType := "stdout"
	endpoint := otelEndpoint

	if otelEndpoint != "" {
		sinkType = "otlp"
	}

	if llmSvc.Spec.Observability != nil && llmSvc.Spec.Observability.Sink != nil {
		sinkType = llmSvc.Spec.Observability.Sink.Type
		if llmSvc.Spec.Observability.Sink.Endpoint != "" {
			endpoint = llmSvc.Spec.Observability.Sink.Endpoint
		}
	}

	// No ConfigMap required when stdout sink and no OTel endpoint (console is the default).
	if sinkType == "stdout" && endpoint == "" {
		return nil
	}

	cfg := VectorConfig{
		Enabled:      true,
		SinkType:     sinkType,
		SinkEndpoint: endpoint,
	}
	cm := BuildVectorConfigMap(llmSvc.Name, llmSvc.Namespace, llmSvc.Spec.Model.Name, cfg)
	if err := controllerutil.SetControllerReference(llmSvc, cm, scheme); err != nil {
		return err
	}

	var existing corev1.ConfigMap
	err := c.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, cm)
	}
	if err != nil {
		return fmt.Errorf("get vector configmap: %w", err)
	}

	if !equality.Semantic.DeepEqual(existing.Data, cm.Data) {
		existing.Data = cm.Data
		return c.Update(ctx, &existing)
	}

	return nil
}
