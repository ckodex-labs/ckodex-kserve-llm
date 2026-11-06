/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package utcp

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidationError represents an error during UTCP manual validation.
type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("utcp validation error: field '%s': %s: %v", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("utcp validation error: field '%s': %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// ValidateManual verifies that a UTCP Manual satisfies the UTCP specification.
func ValidateManual(m *Manual) error {
	if m == nil {
		return &ValidationError{Field: "manual", Message: "manual cannot be nil"}
	}
	if strings.TrimSpace(m.UTCPVersion) == "" {
		return &ValidationError{Field: "utcp_version", Message: "utcp_version is required"}
	}
	if strings.TrimSpace(m.ManualVersion) == "" {
		return &ValidationError{Field: "manual_version", Message: "manual_version is required"}
	}
	if strings.TrimSpace(m.Info.Title) == "" {
		return &ValidationError{Field: "info.title", Message: "info.title is required"}
	}
	return validateTools(m.Tools)
}

func validateTools(tools []Tool) error {
	toolNames := make(map[string]bool)
	for i, t := range tools {
		if strings.TrimSpace(t.Name) == "" {
			return &ValidationError{Field: fmt.Sprintf("tools[%d].name", i), Message: "tool name is required"}
		}
		if toolNames[t.Name] {
			return &ValidationError{Field: fmt.Sprintf("tools[%d].name", i), Message: fmt.Sprintf("duplicate tool name '%s'", t.Name)}
		}
		toolNames[t.Name] = true

		if strings.TrimSpace(t.Description) == "" {
			return &ValidationError{Field: fmt.Sprintf("tools[%d].description", i), Message: "tool description is required"}
		}
		if t.Inputs == nil {
			return &ValidationError{Field: fmt.Sprintf("tools[%d].inputs", i), Message: "tool inputs schema is required"}
		}
		if err := validateToolCallTemplate(t.ToolCallTemplate, i); err != nil {
			return err
		}
	}
	return nil
}

func validateToolCallTemplate(tmpl *ToolCallTemplate, index int) error {
	if tmpl == nil {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template", index), Message: "tool_call_template is required"}
	}
	switch tmpl.CallTemplateType {
	case TemplateTypeHTTP:
		return validateHTTPTemplate(tmpl, index)
	case TemplateTypeGRPC:
		return validateGRPCTemplate(tmpl, index)
	case TemplateTypeCLI:
		return validateCLITemplate(tmpl, index)
	case TemplateTypeWebSocket, TemplateTypeSSE:
		if strings.TrimSpace(tmpl.URL) == "" {
			return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.url", index), Message: "url is required for streaming templates"}
		}
		return nil
	default:
		return &ValidationError{
			Field:   fmt.Sprintf("tools[%d].tool_call_template.call_template_type", index),
			Message: fmt.Sprintf("unsupported call_template_type '%s'", tmpl.CallTemplateType),
		}
	}
}

func validateHTTPTemplate(tmpl *ToolCallTemplate, index int) error {
	if strings.TrimSpace(tmpl.URL) == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.url", index), Message: "url is required for http template"}
	}
	if _, err := url.Parse(tmpl.URL); err != nil {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.url", index), Message: "invalid url format", Err: err}
	}
	method := strings.ToUpper(strings.TrimSpace(tmpl.HTTPMethod))
	if method == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.http_method", index), Message: "http_method is required"}
	}
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return nil
	default:
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.http_method", index), Message: fmt.Sprintf("unsupported http_method '%s'", method)}
	}
}

func validateGRPCTemplate(tmpl *ToolCallTemplate, index int) error {
	if strings.TrimSpace(tmpl.Endpoint) == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.endpoint", index), Message: "endpoint is required for grpc template"}
	}
	if strings.TrimSpace(tmpl.Service) == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.service", index), Message: "service is required for grpc template"}
	}
	if strings.TrimSpace(tmpl.Method) == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.method", index), Message: "method is required for grpc template"}
	}
	return nil
}

func validateCLITemplate(tmpl *ToolCallTemplate, index int) error {
	if strings.TrimSpace(tmpl.Command) == "" {
		return &ValidationError{Field: fmt.Sprintf("tools[%d].tool_call_template.command", index), Message: "command is required for cli template"}
	}
	return nil
}
