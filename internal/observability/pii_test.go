/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package observability_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ckodex-labs/kserve-llm-operator/internal/observability"
)

// ---- Redactor ----------------------------------------------------------------

func TestRedactor_Disabled_ReturnsOriginal(t *testing.T) {
	r := observability.NewRedactor(false)
	details := map[string]string{"field": "123-45-6789"}
	out := r.RedactDetails(details)
	// When disabled, the original map is returned unchanged (no allocation).
	assert.Equal(t, "123-45-6789", out["field"])
}

func TestRedactor_EmptyMap_ReturnsEmpty(t *testing.T) {
	r := observability.NewRedactor(true)
	out := r.RedactDetails(map[string]string{})
	assert.Empty(t, out)
}

func TestRedactor_NilMap_ReturnsNil(t *testing.T) {
	r := observability.NewRedactor(true)
	out := r.RedactDetails(nil)
	assert.Nil(t, out)
}

func TestRedactor_KeysPreserved_ValuesRedacted(t *testing.T) {
	r := observability.NewRedactor(true)
	details := map[string]string{
		"user_email": "alice@example.com",
		"safe_field": "hello world",
	}
	out := r.RedactDetails(details)
	assert.Equal(t, observability.RedactedPlaceholder, out["user_email"])
	assert.Equal(t, "hello world", out["safe_field"])
}

// ---- PII patterns ------------------------------------------------------------

func TestRedactor_SSN_Redacted(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{"123-45-6789", "999-99-9999", "000-12-3456"}
	for _, ssn := range cases {
		t.Run(ssn, func(t *testing.T) {
			assert.Equal(t, observability.RedactedPlaceholder, r.RedactString(ssn))
		})
	}
}

func TestRedactor_Email_Redacted(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{
		"user@example.com",
		"alice.smith+tag@corp.internal",
		"no-reply@sub.domain.co.uk",
	}
	for _, email := range cases {
		assert.Equal(t, observability.RedactedPlaceholder, r.RedactString(email),
			"email %q should be redacted", email)
	}
}

func TestRedactor_Phone_Redacted(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{
		"(416) 555-1234",
		"416-555-1234",
		"416.555.1234",
	}
	for _, phone := range cases {
		assert.Contains(t, r.RedactString(phone), observability.RedactedPlaceholder,
			"phone %q should be redacted", phone)
	}
}

func TestRedactor_IPv4_Redacted(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{"192.168.1.1", "10.0.0.1", "8.8.8.8", "172.16.0.1"}
	for _, ip := range cases {
		assert.Contains(t, r.RedactString(ip), observability.RedactedPlaceholder,
			"IP %q should be redacted", ip)
	}
}

func TestRedactor_BearerToken_Redacted(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{
		"Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig",
		"bearer abc123token",
		"BEARER some.token.here",
	}
	for _, tok := range cases {
		assert.Contains(t, r.RedactString(tok), observability.RedactedPlaceholder,
			"bearer token %q should be redacted", tok)
	}
}

func TestRedactor_CleanString_NotModified(t *testing.T) {
	r := observability.NewRedactor(true)
	cases := []string{
		"hello world",
		"model=llama3",
		"namespace=default",
		"reconcile successful",
		"",
	}
	for _, s := range cases {
		assert.Equal(t, s, r.RedactString(s), "clean string %q must not be modified", s)
	}
}

func TestRedactor_MultiplePatterns_AllRedacted(t *testing.T) {
	r := observability.NewRedactor(true)
	// String containing both email and SSN
	input := "user alice@example.com ssn 123-45-6789 called"
	output := r.RedactString(input)
	assert.NotContains(t, output, "alice@example.com")
	assert.NotContains(t, output, "123-45-6789")
	assert.Contains(t, output, observability.RedactedPlaceholder)
}

func TestRedactor_InContext_Preserved(t *testing.T) {
	r := observability.NewRedactor(true)
	// The surrounding context (non-PII text) should be preserved.
	input := "request from user@example.com completed"
	output := r.RedactString(input)
	assert.True(t, strings.HasPrefix(output, "request from "), "prefix must be preserved")
	assert.True(t, strings.HasSuffix(output, " completed"), "suffix must be preserved")
}

// ---- ContainsPII -------------------------------------------------------------

func TestContainsPII_True(t *testing.T) {
	cases := []string{
		"123-45-6789",       // SSN
		"user@example.com",  // email
		"192.168.0.1",       // IP
		"Bearer token.here", // bearer
	}
	for _, s := range cases {
		assert.True(t, observability.ContainsPII(s), "%q should contain PII", s)
	}
}

func TestContainsPII_False(t *testing.T) {
	cases := []string{
		"reconcile loop completed",
		"model=llama3 namespace=prod",
		"",
		"no-pii-here",
	}
	for _, s := range cases {
		assert.False(t, observability.ContainsPII(s), "%q should not contain PII", s)
	}
}

// ---- SanitizeKey -------------------------------------------------------------

func TestSanitizeKey_Clean(t *testing.T) {
	assert.Equal(t, "model_name", observability.SanitizeKey("model_name"))
	assert.Equal(t, "namespace", observability.SanitizeKey("namespace"))
}

func TestSanitizeKey_NewlineReplaced(t *testing.T) {
	assert.Equal(t, "key_with_newline", observability.SanitizeKey("key\nwith\nnewline"))
}

func TestSanitizeKey_CarriageReturnReplaced(t *testing.T) {
	assert.Equal(t, "key_cr", observability.SanitizeKey("key\rcr"))
}

func TestSanitizeKey_TabReplaced(t *testing.T) {
	assert.Equal(t, "key_tab", observability.SanitizeKey("key\ttab"))
}

func TestSanitizeKey_Empty(t *testing.T) {
	assert.Equal(t, "", observability.SanitizeKey(""))
}
