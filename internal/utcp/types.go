/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package utcp implements the Universal Tool Calling Protocol (UTCP) specification.
// UTCP enables AI agents to discover and invoke tools directly over their native
// protocols (HTTP, gRPC, CLI) without intermediate wrapper proxy servers.
// Reference: https://github.com/universal-tool-calling-protocol/utcp-specification
package utcp

// UTCPVersion defines the supported UTCP specification version.
const UTCPVersion = "1.0.1"

// CallTemplateType defines the transport mechanism for a tool call.
type CallTemplateType string

const (
	TemplateTypeHTTP      CallTemplateType = "http"
	TemplateTypeGRPC      CallTemplateType = "grpc"
	TemplateTypeCLI       CallTemplateType = "cli"
	TemplateTypeWebSocket CallTemplateType = "websocket"
	TemplateTypeSSE       CallTemplateType = "sse"
)

// ManualInfo provides metadata about the UTCP manual and tool provider.
type ManualInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// ToolCallTemplate defines how an agent invokes the tool natively.
type ToolCallTemplate struct {
	CallTemplateType CallTemplateType  `json:"call_template_type"`
	URL              string            `json:"url,omitempty"`
	HTTPMethod       string            `json:"http_method,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	QueryParams      map[string]string `json:"query_params,omitempty"`
	Body             any               `json:"body,omitempty"`
	Service          string            `json:"service,omitempty"`
	Method           string            `json:"method,omitempty"`
	Endpoint         string            `json:"endpoint,omitempty"`
	Proto            string            `json:"proto,omitempty"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
}

// Tool defines an individual tool declared in a UTCP manual.
type Tool struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Inputs           any               `json:"inputs"`
	Output           any               `json:"output,omitempty"`
	ToolCallTemplate *ToolCallTemplate `json:"tool_call_template"`
	Tags             []string          `json:"tags,omitempty"`
}

// Manual represents the root document of a Universal Tool Calling Protocol manual.
type Manual struct {
	UTCPVersion   string     `json:"utcp_version"`
	ManualVersion string     `json:"manual_version"`
	Info          ManualInfo `json:"info"`
	Tools         []Tool     `json:"tools"`
}

// OpenAIFunction represents the function definition format expected by OpenAI / vLLM.
type OpenAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// OpenAITool represents an OpenAI-compatible tool definition.
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// MCPToolDefinition represents the schema consumed by utcp-mcp bridges.
type MCPToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}
