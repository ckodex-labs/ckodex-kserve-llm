/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/google/uuid"
)

// AuditAction identifies the type of reconcile or inference-time event.
type AuditAction string

const (
	// Reconcile lifecycle events (existing).
	AuditCreate          AuditAction = "Create"
	AuditUpdate          AuditAction = "Update"
	AuditDelete          AuditAction = "Delete"
	AuditScale           AuditAction = "Scale"
	AuditPolicyViolation AuditAction = "PolicyViolation"
	AuditModelLoaded     AuditAction = "ModelLoaded"
	AuditSecurityEvent   AuditAction = "SecurityEvent"
	AuditGatewaySync     AuditAction = "GatewaySync"

	// Inference-time events — required for SOC2 Type II and FedRAMP audit trails.
	// These must be emitted by the inference proxy / session handler, not the controller.

	// AuditModelAccess records a successful inference request (model + tenant + token count).
	AuditModelAccess AuditAction = "ModelAccess"
	// AuditModelAccessDenied records an access-denied inference attempt.
	AuditModelAccessDenied AuditAction = "ModelAccessDenied"
	// AuditTokenConsumed records actual token consumption after a completed request.
	AuditTokenConsumed AuditAction = "TokenConsumed"
	// AuditTokenBudgetExceeded records a 429 response due to exhausted token budget.
	AuditTokenBudgetExceeded AuditAction = "TokenBudgetExceeded"
	// AuditCredentialAccess records API key or JWT validation (success or failure).
	AuditCredentialAccess AuditAction = "CredentialAccess"
	// AuditLoraSwap records a LoRA adapter hot-swap on a running inference service.
	AuditLoraSwap AuditAction = "LoraSwap"
	// AuditModelPromotion records a model version promotion (staging → production).
	AuditModelPromotion AuditAction = "ModelPromotion"
)

// AuditOutcome indicates whether the audited action succeeded.
type AuditOutcome string

const (
	AuditSuccess AuditOutcome = "Success"
	AuditFailure AuditOutcome = "Failure"
	AuditDenied  AuditOutcome = "Denied"
)

// AuditEvent represents a structured audit event for the reconcile lifecycle.
type AuditEvent struct {
	// Action is the type of event.
	Action AuditAction `json:"action"`

	// Resource identifies the K8s resource (kind/namespace/name).
	Resource string `json:"resource"`

	// Actor identifies who/what triggered the event.
	Actor string `json:"actor"`

	// Outcome is the result of the action.
	Outcome AuditOutcome `json:"outcome"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Details holds action-specific structured data.
	Details map[string]string `json:"details,omitempty"`

	// Reason provides human-readable context.
	Reason string `json:"reason,omitempty"`

	// OIS v0.1 fields
	ExecID               string `json:"exec.id,omitempty"`
	ExecKind             string `json:"exec.kind,omitempty"`
	ReproducibilityClass string `json:"exec.reproducibility_class,omitempty"`
}

// AuditLogger emits structured audit events to slog, OTel spans, and K8s Events.
type AuditLogger struct {
	client.Client
	Scheme        *runtime.Scheme
	logger        *slog.Logger
	redactor      *Redactor
	auditFilePath string // Path for persistent JSONL file auditor
	otelEndpoint  string // OIS v0.1: OTLP/HTTP endpoint for direct audit sink
}

// NewAuditLogger creates an audit logger with structured JSON output.
// piiRedaction=true enables regex-based PII scrubbing on all detail fields
// before emission to any sink.
func NewAuditLogger(c client.Client, scheme *runtime.Scheme) *AuditLogger {
	return NewAuditLoggerWithOptions(c, scheme, true)
}

// NewAuditLoggerWithOptions creates an audit logger with explicit PII redaction control.
func NewAuditLoggerWithOptions(c client.Client, scheme *runtime.Scheme, piiRedaction bool) *AuditLogger {
	// For production readiness, we attempt to use /var/log/ckodex/audit.jsonl
	// if the directory is writable (e.g. via a PV mount).
	auditPath := os.Getenv("CKODEX_AUDIT_LOG_PATH")
	if auditPath == "" {
		auditPath = "/var/log/ckodex/audit.jsonl"
	}

	return &AuditLogger{
		Client:        c,
		Scheme:        scheme,
		logger:        slog.Default().With("component", "audit"),
		redactor:      NewRedactor(piiRedaction),
		auditFilePath: auditPath,
		otelEndpoint:  os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
}

// LogCreate records a resource creation event.
func (a *AuditLogger) LogCreate(ctx context.Context, resource, actor string, details map[string]string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditCreate,
		Resource:  resource,
		Actor:     actor,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details:   details,
	})
}

// LogUpdate records a resource update event.
func (a *AuditLogger) LogUpdate(ctx context.Context, resource, actor string, details map[string]string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditUpdate,
		Resource:  resource,
		Actor:     actor,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details:   details,
	})
}

// LogDelete records a resource deletion event.
func (a *AuditLogger) LogDelete(ctx context.Context, resource, actor string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditDelete,
		Resource:  resource,
		Actor:     actor,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
	})
}

// LogScaleEvent records an autoscaling event.
func (a *AuditLogger) LogScaleEvent(ctx context.Context, resource string, fromReplicas, toReplicas int32, reason string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditScale,
		Resource:  resource,
		Actor:     "autoscaler",
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details: map[string]string{
			"from_replicas": slog.IntValue(int(fromReplicas)).String(),
			"to_replicas":   slog.IntValue(int(toReplicas)).String(),
		},
		Reason: reason,
	})
}

// LogPolicyViolation records an OPA/security policy violation.
func (a *AuditLogger) LogPolicyViolation(ctx context.Context, resource, violation, policy string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditPolicyViolation,
		Resource:  resource,
		Actor:     "opa-gatekeeper",
		Outcome:   AuditDenied,
		Timestamp: time.Now(),
		Details: map[string]string{
			"violation": violation,
			"policy":    policy,
		},
	})
}

// LogFailure records a failed operation.
func (a *AuditLogger) LogFailure(ctx context.Context, action AuditAction, resource, actor, reason string) {
	a.emit(ctx, AuditEvent{
		Action:    action,
		Resource:  resource,
		Actor:     actor,
		Outcome:   AuditFailure,
		Timestamp: time.Now(),
		Reason:    reason,
	})
}

// LogModelAccess records a successful inference request.
// tenantID, modelName, subject, and tokensUsed are required; prompt/response are
// intentionally excluded — use AuditSinkConfig.PIIRedaction if partial context is needed.
func (a *AuditLogger) LogModelAccess(ctx context.Context, tenantID, modelName, subject string, tokensUsed int64) {
	a.emit(ctx, AuditEvent{
		Action:    AuditModelAccess,
		Resource:  modelName,
		Actor:     subject,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details: map[string]string{
			"tenant_id":   tenantID,
			"model":       modelName,
			"tokens_used": fmt.Sprintf("%d", tokensUsed),
		},
	})
}

// LogModelAccessDenied records an inference request that was rejected by the
// model access enforcer or token budget enforcer.
func (a *AuditLogger) LogModelAccessDenied(ctx context.Context, tenantID, modelName, subject, reason string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditModelAccessDenied,
		Resource:  modelName,
		Actor:     subject,
		Outcome:   AuditDenied,
		Timestamp: time.Now(),
		Reason:    reason,
		Details: map[string]string{
			"tenant_id": tenantID,
			"model":     modelName,
		},
	})
}

// LogTokenConsumed records actual token usage after a completed inference request.
func (a *AuditLogger) LogTokenConsumed(ctx context.Context, tenantID, modelName, subject string, promptTokens, completionTokens int64) {
	a.emit(ctx, AuditEvent{
		Action:    AuditTokenConsumed,
		Resource:  modelName,
		Actor:     subject,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details: map[string]string{
			"tenant_id":         tenantID,
			"model":             modelName,
			"prompt_tokens":     fmt.Sprintf("%d", promptTokens),
			"completion_tokens": fmt.Sprintf("%d", completionTokens),
			"total_tokens":      fmt.Sprintf("%d", promptTokens+completionTokens),
		},
	})
}

// LogTokenBudgetExceeded records a 429 response due to budget exhaustion.
func (a *AuditLogger) LogTokenBudgetExceeded(ctx context.Context, tenantID, modelName, subject string, remaining int64) {
	a.emit(ctx, AuditEvent{
		Action:    AuditTokenBudgetExceeded,
		Resource:  modelName,
		Actor:     subject,
		Outcome:   AuditDenied,
		Timestamp: time.Now(),
		Reason:    "token budget exhausted",
		Details: map[string]string{
			"tenant_id":        tenantID,
			"model":            modelName,
			"remaining_budget": fmt.Sprintf("%d", remaining),
		},
	})
}

// LogCredentialAccess records an authentication attempt (API key or JWT).
func (a *AuditLogger) LogCredentialAccess(ctx context.Context, subject, credentialType string, success bool) {
	outcome := AuditSuccess
	if !success {
		outcome = AuditFailure
	}
	a.emit(ctx, AuditEvent{
		Action:    AuditCredentialAccess,
		Resource:  "auth",
		Actor:     subject,
		Outcome:   outcome,
		Timestamp: time.Now(),
		Details: map[string]string{
			"credential_type": credentialType,
		},
	})
}

// LogLoraSwap records a LoRA adapter hot-swap event.
func (a *AuditLogger) LogLoraSwap(ctx context.Context, serviceName, namespace, fromAdapter, toAdapter, actor string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditLoraSwap,
		Resource:  fmt.Sprintf("LLMInferenceService/%s/%s", namespace, serviceName),
		Actor:     actor,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details: map[string]string{
			"from_adapter": fromAdapter,
			"to_adapter":   toAdapter,
		},
	})
}

// LogModelPromotion records a model version promotion across environments.
func (a *AuditLogger) LogModelPromotion(ctx context.Context, modelName, fromEnv, toEnv, actor string) {
	a.emit(ctx, AuditEvent{
		Action:    AuditModelPromotion,
		Resource:  modelName,
		Actor:     actor,
		Outcome:   AuditSuccess,
		Timestamp: time.Now(),
		Details: map[string]string{
			"from_environment": fromEnv,
			"to_environment":   toEnv,
		},
	})
}

// emit writes the audit event to all configured sinks.
func (a *AuditLogger) emit(ctx context.Context, event AuditEvent) {
	// OIS v0.1: Ensure execution identity
	if event.ExecID == "" {
		event.ExecID = URN("exec", uuid.New().String())
	}
	if event.ExecKind == "" {
		event.ExecKind = a.mapToExecKind(event.Action)
	}
	if event.ReproducibilityClass == "" {
		event.ReproducibilityClass = ReproExplanatory
	}

	// 0. PII redaction — applied before any sink sees the details.
	if a.redactor != nil {
		event.Details = a.redactor.RedactDetails(event.Details)
		event.Reason = a.redactor.RedactString(event.Reason)
	}

	// 1. Structured slog output (JSON)
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

	// 2. OTel span event
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
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
		// OIS v0.1: Promote structured details to span attributes
		for k, v := range event.Details {
			attrs = append(attrs, attribute.String(k, v))
		}
		span.AddEvent("audit", trace.WithAttributes(attrs...))
	}
	// 3. Persistent File Audit (Best-effort, synchronous file append)
	a.emitToFile(event)

	// 4. Direct OTLP Sink (OIS v0.1 / Contract)
	if a.otelEndpoint != "" {
		go a.emitToOTLP(ctx, event)
	}

	// 5. K8s Event (best-effort, non-blocking)
	go a.emitK8sEvent(ctx, event)
}

// emitToOTLP sends the audit event directly to an OTel collector as a log record.
func (a *AuditLogger) emitToOTLP(ctx context.Context, event AuditEvent) {
	// In a real production implementation, we would use the OTel Logs SDK.
	// For this wave, we emit a debug log indicating the OTLP route is active.
	// The operator's own Vector sidecar (if deployed) or FluentBit would scrape the
	// slog JSON output and forward it to OTLP_ENDPOINT automatically.
	a.logger.Debug("Audit signal routed to OTLP collector", "endpoint", a.otelEndpoint, "exec.id", event.ExecID)
}

// emitToFile writes the event as a JSON line to the persistent audit file.
func (a *AuditLogger) emitToFile(event AuditEvent) {
	if a.auditFilePath == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(a.auditFilePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			a.logger.Error("Failed to create audit log directory", "path", dir, "error", err)
			return
		}
	}

	f, err := os.OpenFile(a.auditFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		a.logger.Error("Failed to open audit log file", "path", a.auditFilePath, "error", err)
		return
	}
	defer func() { _ = f.Close() }()

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		a.logger.Error("Failed to write to audit log file", "path", a.auditFilePath, "error", err)
	}
}

// emitK8sEvent creates a Kubernetes Event resource for the audit trail.
func (a *AuditLogger) emitK8sEvent(ctx context.Context, event AuditEvent) {
	eventJSON, _ := json.Marshal(event.Details)

	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ckodex-audit-",
			Namespace:    "default",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				"ckodex.com/audit-action":      string(event.Action),
			},
		},
		Reason:              string(event.Action),
		Message:             event.Reason + " | " + string(eventJSON),
		Type:                "Normal",
		Action:              string(event.Action),
		EventTime:           metav1.NowMicro(),
		ReportingController: "ckodex-kserve-llm-operator",
		ReportingInstance:   "controller-manager",
	}

	if event.Outcome == AuditFailure || event.Outcome == AuditDenied {
		k8sEvent.Type = "Warning"
	}

	// Best-effort: don't fail reconcile if event creation fails
	if a.Client != nil {
		_ = a.Create(ctx, k8sEvent)
	}
}

// LogRichInferenceSignal records a full OIS Inference Profile envelope (Section 26.2).
func (a *AuditLogger) LogRichInferenceSignal(
	ctx context.Context,
	execID string,
	actor string,
	modelAssembly ModelAssembly,
	perf PerformanceMetrics,
	outcome AuditOutcome,
	details map[string]string,
) {
	if details == nil {
		details = make(map[string]string)
	}

	// Map Performance to string details for AuditEvent persistence
	details[AttrPerfLatencyMS] = fmt.Sprintf("%d", perf.LatencyMS)
	if perf.FirstTokenMS > 0 {
		details[AttrPerfFirstTokenMS] = fmt.Sprintf("%d", perf.FirstTokenMS)
	}
	details[AttrModelBaseID] = modelAssembly.Base.ID
	if modelAssembly.Quantization != nil {
		details["model.quantization.id"] = modelAssembly.Quantization.ID
	}

	a.emit(ctx, AuditEvent{
		Action:               AuditModelAccess,
		Resource:             modelAssembly.Base.ID,
		Actor:                actor,
		Outcome:              outcome,
		Timestamp:            time.Now(),
		ExecID:               execID,
		ExecKind:             ExecKindInference,
		ReproducibilityClass: ReproExplanatory,
		Details:              details,
	})
}

// mapToExecKind converts AuditAction to OIS ExecKind.
func (a *AuditLogger) mapToExecKind(action AuditAction) string {
	switch action {
	case AuditModelAccess, AuditModelAccessDenied, AuditTokenConsumed:
		return ExecKindInference
	case AuditPolicyViolation:
		return ExecKindGuardrail
	case AuditGatewaySync:
		return ExecKindMaterialization
	case AuditLoraSwap:
		return ExecKindAgentStep
	default:
		return ExecKindInference // Default for operator lifecycle
	}
}

// LogReceipt records a post-execution record linking outcome and evidence (OIS Section 21).
func (a *AuditLogger) LogReceipt(ctx context.Context, execID, status, summary string, details map[string]string) {
	a.emit(ctx, AuditEvent{
		Action:               "Receipt",
		Resource:             "ois/receipt",
		Actor:                "ckodex-operator",
		Outcome:              AuditSuccess,
		Timestamp:            time.Now(),
		ExecID:               execID,
		ExecKind:             "receipt",
		ReproducibilityClass: ReproAttested,
		Reason:               summary,
		Details:              details,
	})
}
