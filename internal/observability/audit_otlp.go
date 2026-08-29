/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otelLog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	auditOTLPPath          = "/v1/logs"
	auditOTLPExportTimeout = 5 * time.Second
	auditOTLPBatchInterval = 500 * time.Millisecond
	auditOTLPMaxQueueSize  = 2048
	auditOTLPMaxBatchSize  = 128
)

// configureOTLP creates the bounded audit log pipeline. The endpoint is
// explicit so audit records do not accidentally follow a metrics-only sink.
func (a *AuditLogger) configureOTLP(ctx context.Context, rawEndpoint string) error {
	endpoint, err := normalizeAuditOTLPEndpoint(rawEndpoint)
	if err != nil {
		return err
	}

	options := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(endpoint),
		otlploghttp.WithTimeout(auditOTLPExportTimeout),
	}
	if strings.HasPrefix(endpoint, "http://") {
		options = append(options, otlploghttp.WithInsecure())
	}
	exporter, err := otlploghttp.New(ctx, options...)
	if err != nil {
		return fmt.Errorf("create OTLP audit exporter: %w", err)
	}

	processor := sdklog.NewBatchProcessor(
		&auditOTLPExporter{delegate: exporter, logger: a.logger},
		sdklog.WithExportInterval(auditOTLPBatchInterval),
		sdklog.WithExportTimeout(auditOTLPExportTimeout),
		sdklog.WithMaxQueueSize(auditOTLPMaxQueueSize),
		sdklog.WithExportMaxBatchSize(auditOTLPMaxBatchSize),
	)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "ckodex-kserve-llm-operator"),
			attribute.String("service.namespace", "ckodex"),
		)),
	)
	a.otelProvider = provider
	a.otelLogger = provider.Logger(InstrumentationName)
	return nil
}

func normalizeAuditOTLPEndpoint(rawEndpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawEndpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("audit OTLP endpoint must be an absolute http(s) URL: %q", rawEndpoint)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("audit OTLP endpoint scheme must be http or https: %q", parsed.Scheme)
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = auditOTLPPath
	}
	return parsed.String(), nil
}

func (a *AuditLogger) emitToOTLPLog(ctx context.Context, event AuditEvent) {
	if a.otelLogger == nil {
		return
	}
	var record otelLog.Record
	record.SetTimestamp(event.Timestamp)
	record.SetObservedTimestamp(time.Now())
	record.SetEventName("ckodex.audit")
	record.SetSeverity(otelLog.SeverityInfo)
	record.SetSeverityText("INFO")
	record.SetBody(attribute.StringValue(event.Reason))
	record.AddAttributes(auditSpanAttributes(event)...)
	a.otelLogger.Emit(ctx, record)
}

type auditOTLPExporter struct {
	delegate sdklog.Exporter
	logger   *slog.Logger
}

func (e *auditOTLPExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.delegate.Export(ctx, records)
	if err != nil && e.logger != nil {
		e.logger.Error("OTLP audit export failed", "records", len(records), "error", err)
	}
	return err
}

func (e *auditOTLPExporter) ForceFlush(ctx context.Context) error {
	return e.delegate.ForceFlush(ctx)
}

func (e *auditOTLPExporter) Shutdown(ctx context.Context) error {
	return e.delegate.Shutdown(ctx)
}
