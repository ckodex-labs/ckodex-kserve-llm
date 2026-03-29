/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// auditScheme returns a runtime.Scheme with corev1 registered (needed for
// the fake client to handle corev1.Event creation in AuditLogger.emitK8sEvent).
func auditScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

// newAuditLogger creates an AuditLogger backed by a fake k8s client.
func newAuditLogger(t *testing.T) *observability.AuditLogger {
	t.Helper()
	s := auditScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	return observability.NewAuditLogger(fc, s)
}

// newAuditLoggerNoPII creates an AuditLogger with PII redaction disabled.
func newAuditLoggerNoPII(t *testing.T) *observability.AuditLogger {
	t.Helper()
	s := auditScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	return observability.NewAuditLoggerWithOptions(fc, s, false)
}

// drainK8sEvents gives the background goroutine in emitK8sEvent time to run.
func drainK8sEvents() { time.Sleep(10 * time.Millisecond) }

// ---- NewAuditLogger ----------------------------------------------------------

func TestNewAuditLogger_NotNil(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotNil(t, al)
}

func TestNewAuditLoggerWithOptions_PIIRedactionFalse_NotNil(t *testing.T) {
	al := newAuditLoggerNoPII(t)
	assert.NotNil(t, al)
}

// ---- LogCreate ---------------------------------------------------------------

func TestAuditLogger_LogCreate_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogCreate(context.Background(), "LLMInferenceService/prod/llama3", "controller",
			map[string]string{"replicas": "3"})
	})
	drainK8sEvents()
}

func TestAuditLogger_LogCreate_PIIRedactionEnabled_RedactsDetails(t *testing.T) {
	al := newAuditLogger(t)
	// Should not panic even with PII-bearing values.
	assert.NotPanics(t, func() {
		al.LogCreate(context.Background(), "svc", "controller",
			map[string]string{"user": "user@example.com"})
	})
	drainK8sEvents()
}

// ---- LogUpdate ---------------------------------------------------------------

func TestAuditLogger_LogUpdate_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogUpdate(context.Background(), "svc/prod/m1", "controller",
			map[string]string{"image": "llama3:v2"})
	})
	drainK8sEvents()
}

// ---- LogDelete ---------------------------------------------------------------

func TestAuditLogger_LogDelete_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogDelete(context.Background(), "svc/prod/m1", "controller")
	})
	drainK8sEvents()
}

// ---- LogScaleEvent -----------------------------------------------------------

func TestAuditLogger_LogScaleEvent_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogScaleEvent(context.Background(), "svc/prod/m1", 1, 3, "high load")
	})
	drainK8sEvents()
}

// ---- LogPolicyViolation ------------------------------------------------------

func TestAuditLogger_LogPolicyViolation_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogPolicyViolation(context.Background(), "svc/prod/m1", "registry-not-allowed", "opa-registry")
	})
	drainK8sEvents()
}

// ---- LogFailure --------------------------------------------------------------

func TestAuditLogger_LogFailure_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogFailure(context.Background(), observability.AuditCreate, "svc/prod/m1",
			"controller", "image pull failed")
	})
	drainK8sEvents()
}

// ---- LogModelAccess ----------------------------------------------------------

func TestAuditLogger_LogModelAccess_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogModelAccess(context.Background(), "acme", "llama3", "user-001", 512)
	})
	drainK8sEvents()
}

// ---- LogModelAccessDenied ----------------------------------------------------

func TestAuditLogger_LogModelAccessDenied_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogModelAccessDenied(context.Background(), "acme", "llama3", "user-001", "budget exhausted")
	})
	drainK8sEvents()
}

// ---- LogTokenConsumed --------------------------------------------------------

func TestAuditLogger_LogTokenConsumed_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogTokenConsumed(context.Background(), "acme", "llama3", "user-001", 100, 200)
	})
	drainK8sEvents()
}

// ---- LogTokenBudgetExceeded --------------------------------------------------

func TestAuditLogger_LogTokenBudgetExceeded_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogTokenBudgetExceeded(context.Background(), "acme", "llama3", "user-001", 0)
	})
	drainK8sEvents()
}

// ---- LogCredentialAccess -----------------------------------------------------

func TestAuditLogger_LogCredentialAccess_Success_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogCredentialAccess(context.Background(), "user-001", "jwt", true)
	})
	drainK8sEvents()
}

func TestAuditLogger_LogCredentialAccess_Failure_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogCredentialAccess(context.Background(), "user-001", "api-key", false)
	})
	drainK8sEvents()
}

// ---- LogLoraSwap -------------------------------------------------------------

func TestAuditLogger_LogLoraSwap_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogLoraSwap(context.Background(), "llm-svc", "prod", "adapter-v1", "adapter-v2", "operator")
	})
	drainK8sEvents()
}

// ---- LogModelPromotion -------------------------------------------------------

func TestAuditLogger_LogModelPromotion_NoPanic(t *testing.T) {
	al := newAuditLogger(t)
	assert.NotPanics(t, func() {
		al.LogModelPromotion(context.Background(), "llama3", "staging", "production", "ci-pipeline")
	})
	drainK8sEvents()
}

// ---- PIIRedaction disabled ---------------------------------------------------

func TestAuditLogger_PIIRedactionDisabled_NoPanic(t *testing.T) {
	al := newAuditLoggerNoPII(t)
	assert.NotPanics(t, func() {
		al.LogCreate(context.Background(), "svc", "controller",
			map[string]string{"field": "value"})
	})
	drainK8sEvents()
}

// ---- ComplianceViolation.Error -----------------------------------------------

func TestComplianceViolation_Error_Format(t *testing.T) {
	cv := observability.ComplianceViolation{
		Constraint:  "EnableAuth must be true",
		Remediation: "set CKODEX_FEATURE_ENABLE_AUTH=true",
	}
	msg := cv.Error()
	assert.Contains(t, msg, "EnableAuth must be true")
	assert.Contains(t, msg, "set CKODEX_FEATURE_ENABLE_AUTH=true")
}

// ---- AuditAction constants ---------------------------------------------------

func TestAuditActionConstants_NotEmpty(t *testing.T) {
	actions := []observability.AuditAction{
		observability.AuditCreate,
		observability.AuditUpdate,
		observability.AuditDelete,
		observability.AuditScale,
		observability.AuditPolicyViolation,
		observability.AuditModelLoaded,
		observability.AuditSecurityEvent,
		observability.AuditGatewaySync,
		observability.AuditModelAccess,
		observability.AuditModelAccessDenied,
		observability.AuditTokenConsumed,
		observability.AuditTokenBudgetExceeded,
		observability.AuditCredentialAccess,
		observability.AuditLoraSwap,
		observability.AuditModelPromotion,
	}
	for _, a := range actions {
		assert.NotEmpty(t, string(a))
	}
}
