/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/google/uuid"
)

func (a *AuditLogger) prepareAuditEvent(event AuditEvent) AuditEvent {
	if event.ExecID == "" {
		event.ExecID = URN("exec", uuid.New().String())
	}
	if event.ExecKind == "" {
		event.ExecKind = a.mapToExecKind(event.Action)
	}
	if event.ReproducibilityClass == "" {
		event.ReproducibilityClass = ReproExplanatory
	}
	return event
}

func (a *AuditLogger) redactAuditEvent(event AuditEvent) AuditEvent {
	if a.redactor == nil {
		return event
	}
	event.Details = a.redactor.RedactDetails(event.Details)
	event.Reason = a.redactor.RedactString(event.Reason)
	return event
}

func (a *AuditLogger) emitToStructuredLog(ctx context.Context, event AuditEvent) {
	a.logger.LogAttrs(ctx, slog.LevelInfo, "audit_event",
		slog.String("action", string(event.Action)),
		slog.String("resource", event.Resource),
		slog.String("actor", event.Actor),
		slog.String("outcome", string(event.Outcome)),
		slog.Time("timestamp", event.Timestamp),
		slog.String("reason", event.Reason),
		slog.String(AttrExecID, event.ExecID),
		slog.String(AttrExecKind, event.ExecKind),
	)
}

func (a *AuditLogger) emitToOTelSpan(ctx context.Context, event AuditEvent) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	attrs := auditSpanAttributes(event)
	span.AddEvent("audit", trace.WithAttributes(attrs...))
}

func auditSpanAttributes(event AuditEvent) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("audit.action", string(event.Action)),
		attribute.String("audit.resource", event.Resource),
		attribute.String("audit.actor", event.Actor),
		attribute.String("audit.outcome", string(event.Outcome)),
		attribute.String(AttrExecID, event.ExecID),
		attribute.String(AttrExecKind, event.ExecKind),
		attribute.String(AttrExecReproClass, event.ReproducibilityClass),
	}
	if event.Reason != "" {
		attrs = append(attrs, attribute.String("audit.reason", event.Reason))
	}
	for key, value := range event.Details {
		attrs = append(attrs, attribute.String(key, value))
	}
	return attrs
}

func (a *AuditLogger) reportUnavailableOTLP(event AuditEvent) {
	if a.otelEndpoint == "" {
		return
	}
	a.logger.Error("direct OTLP audit export is unavailable; event was not exported",
		"endpoint", a.otelEndpoint,
		"exec.id", event.ExecID,
	)
}
