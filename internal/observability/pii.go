/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability

import (
	"regexp"
	"strings"
)

// RedactedPlaceholder replaces detected PII in audit details.
const RedactedPlaceholder = "[REDACTED]"

// piiPattern holds a compiled pattern and a human-readable name for logging.
type piiPattern struct {
	name    string
	pattern *regexp.Regexp
}

// piiPatterns is the default set of patterns scanned in audit detail values.
// Patterns are intentionally conservative — false positives produce redactions;
// false negatives produce leaks. Err on the side of redaction.
var piiPatterns = []piiPattern{
	{
		name:    "ssn",
		pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	},
	{
		name: "credit_card",
		// Luhn-like 13-19 digit sequences (space or dash separated groups allowed).
		pattern: regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`),
	},
	{
		name:    "email",
		pattern: regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
	},
	{
		name: "phone_us",
		// US phone: (xxx) xxx-xxxx, xxx-xxx-xxxx, +1xxxxxxxxxx
		pattern: regexp.MustCompile(`(?:\+1[-.\s]?)?\(?\d{3}\)?[-.\s]\d{3}[-.\s]\d{4}\b`),
	},
	{
		name: "ipv4",
		// Internal/private IPs are not PII but external IPs can identify users.
		// We redact all IPs to be safe; infrastructure logs keep the originals.
		pattern: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
	},
	{
		name: "bearer_token",
		// Catches accidentally-logged Authorization header values.
		pattern: regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`),
	},
}

// Redactor applies PII redaction to audit event details.
// It is safe to call from multiple goroutines (patterns are read-only after init).
type Redactor struct {
	enabled  bool
	patterns []piiPattern
}

// NewRedactor creates a Redactor. When enabled=false, RedactDetails is a no-op.
func NewRedactor(enabled bool) *Redactor {
	return &Redactor{
		enabled:  enabled,
		patterns: piiPatterns,
	}
}

// RedactDetails returns a copy of details with PII values replaced by [REDACTED].
// Keys are never redacted — only values. If pii redaction is disabled the original
// map is returned unchanged (no copy).
func (r *Redactor) RedactDetails(details map[string]string) map[string]string {
	if !r.enabled || len(details) == 0 {
		return details
	}

	out := make(map[string]string, len(details))
	for k, v := range details {
		out[k] = r.redactString(v)
	}
	return out
}

// RedactString redacts PII from a single string value.
func (r *Redactor) RedactString(s string) string {
	if !r.enabled {
		return s
	}
	return r.redactString(s)
}

func (r *Redactor) redactString(s string) string {
	if s == "" {
		return s
	}
	result := s
	for _, p := range r.patterns {
		result = p.pattern.ReplaceAllString(result, RedactedPlaceholder)
	}
	return result
}

// ContainsPII returns true if the string contains any pattern match.
// Useful for deciding whether to drop a detail entirely vs. redact it.
func ContainsPII(s string) bool {
	for _, p := range piiPatterns {
		if p.pattern.MatchString(s) {
			return true
		}
	}
	return false
}

// SanitizeKey removes characters that could cause log injection (newlines, tabs).
func SanitizeKey(k string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return '_'
		}
		return r
	}, k)
}
