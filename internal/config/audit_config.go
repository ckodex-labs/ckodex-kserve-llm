/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package config

// AuditSinkConfig configures where and how audit events are persisted.
type AuditSinkConfig struct {
	// Type selects the audit sink backend.
	// Supported values: "stdout" (default), "file", "otlp-log".
	// +kubebuilder:validation:Enum=stdout;file;otlp-log
	Type string `json:"type"`

	// OTLPEndpoint is the OTLP/HTTP logs endpoint used when configured.
	// It must include an http:// or https:// scheme and defaults to disabled.
	// +optional
	OTLPEndpoint string `json:"otlpEndpoint,omitempty"`

	// FilePath is the log file path when Type="file".
	// The file is appended-to and rotated at midnight UTC.
	// +optional
	FilePath string `json:"filePath,omitempty"`

	// RetentionDays is how long audit log files are retained before deletion.
	// 0 = keep forever (default).
	RetentionDays int `json:"retentionDays"`

	// PIIRedaction enables regex-based PII detection and redaction in audit
	// event details before they are written to any sink.
	PIIRedaction bool `json:"piiRedaction"`
}
