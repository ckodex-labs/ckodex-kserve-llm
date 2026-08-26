/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// emitToFile writes the event as a JSON line to the persistent audit file.
func (a *AuditLogger) emitToFile(event AuditEvent) {
	if a.auditFilePath == "" {
		return
	}

	dir := filepath.Dir(a.auditFilePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0750); err != nil {
			a.logger.Error("Failed to create audit log directory", "path", dir, "error", err)
			return
		}
	}

	f, err := os.OpenFile(a.auditFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		a.logger.Error("Failed to open audit log file", "path", a.auditFilePath, "error", err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			a.logger.Error("Failed to close audit log file", "path", a.auditFilePath, "error", err)
		}
	}()

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
	eventJSON, err := json.Marshal(event.Details)
	if err != nil {
		a.logger.Error("failed to encode Kubernetes audit event details", "error", err)
		eventJSON = []byte("{}")
	}
	namespace, involvedObject := auditObjectReference(event.Resource)

	k8sEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ckodex-audit-",
			Namespace:    namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ckodex-kserve-llm-operator",
				"ckodex.com/audit-action":      string(event.Action),
			},
		},
		InvolvedObject:      involvedObject,
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

	// Best-effort: don't fail reconcile if event creation fails, but never hide
	// the failure from the audit stream.
	if a.Client != nil {
		if err := a.Create(ctx, k8sEvent); err != nil {
			a.logger.Error("failed to create Kubernetes audit event",
				"action", event.Action,
				"resource", event.Resource,
				"error", err,
			)
		}
	}
}

func auditObjectReference(resource string) (string, corev1.ObjectReference) {
	parts := strings.Split(resource, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "default", corev1.ObjectReference{}
	}
	return parts[1], corev1.ObjectReference{
		APIVersion: "serving.ckodex.com/v1alpha2",
		Kind:       parts[0],
		Namespace:  parts[1],
		Name:       parts[2],
	}
}
