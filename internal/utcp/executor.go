/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package utcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// ToolExecutionReceipt contains the evidence of a native UTCP tool execution.
type ToolExecutionReceipt struct {
	ToolName     string           `json:"tool_name"`
	TemplateType CallTemplateType `json:"template_type"`
	Duration     time.Duration    `json:"duration_ms"`
	StatusCode   int              `json:"status_code,omitempty"`
	Output       any              `json:"output"`
	ExecutedAt   time.Time        `json:"executed_at"`
}

// ExecuteTool invokes a UTCP tool according to its ToolCallTemplate.
func ExecuteTool(ctx context.Context, tool *Tool, inputs map[string]any) (*ToolExecutionReceipt, error) {
	if tool == nil {
		return nil, fmt.Errorf("tool cannot be nil")
	}
	if tool.ToolCallTemplate == nil {
		return nil, fmt.Errorf("tool %q has no tool_call_template", tool.Name)
	}

	start := time.Now()
	var (
		output     any
		statusCode int
		err        error
	)

	switch tool.ToolCallTemplate.CallTemplateType {
	case TemplateTypeHTTP:
		output, statusCode, err = executeHTTP(ctx, tool.ToolCallTemplate, inputs)
	case TemplateTypeCLI:
		output, err = executeCLI(ctx, tool.ToolCallTemplate, inputs)
	default:
		return nil, fmt.Errorf("execution for template type %q not implemented", tool.ToolCallTemplate.CallTemplateType)
	}

	if err != nil {
		return nil, fmt.Errorf("executing tool %q: %w", tool.Name, err)
	}

	return &ToolExecutionReceipt{
		ToolName:     tool.Name,
		TemplateType: tool.ToolCallTemplate.CallTemplateType,
		Duration:     time.Since(start),
		StatusCode:   statusCode,
		Output:       output,
		ExecutedAt:   start,
	}, nil
}

func executeHTTP(ctx context.Context, tmpl *ToolCallTemplate, inputs map[string]any) (any, int, error) {
	bodyBytes, err := json.Marshal(inputs)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling inputs: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(tmpl.HTTPMethod), tmpl.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("creating http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range tmpl.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	var parsed any
	if err := json.Unmarshal(respBytes, &parsed); err == nil {
		return parsed, resp.StatusCode, nil
	}
	return string(respBytes), resp.StatusCode, nil
}

func executeCLI(ctx context.Context, tmpl *ToolCallTemplate, inputs map[string]any) (any, error) {
	args := append([]string(nil), tmpl.Args...)
	inputBytes, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshaling cli inputs: %w", err)
	}

	//nolint:gosec // Subprocess invocation is driven by validated UTCP CLI template under active execution lease
	cmd := exec.CommandContext(ctx, tmpl.Command, args...)
	cmd.Stdin = bytes.NewReader(inputBytes)
	for k, v := range tmpl.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("cli command failed: %w", err)
	}

	var parsed any
	if err := json.Unmarshal(out, &parsed); err == nil {
		return parsed, nil
	}
	return string(out), nil
}
