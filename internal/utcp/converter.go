/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package utcp

import (
	"encoding/json"
	"fmt"
)

// ToOpenAITools converts all tools in a UTCP manual into OpenAI/vLLM compatible tool objects.
func ToOpenAITools(m *Manual) []OpenAITool {
	if m == nil {
		return nil
	}
	res := make([]OpenAITool, 0, len(m.Tools))
	for _, t := range m.Tools {
		res = append(res, ToolToOpenAI(&t))
	}
	return res
}

// ToolToOpenAI converts a single UTCP tool into an OpenAITool.
func ToolToOpenAI(t *Tool) OpenAITool {
	if t == nil {
		return OpenAITool{}
	}
	return OpenAITool{
		Type: "function",
		Function: OpenAIFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Inputs,
		},
	}
}

// ToMCPTools converts a UTCP manual into a list of MCP tool definitions for utcp-mcp bridges.
func ToMCPTools(m *Manual) []MCPToolDefinition {
	if m == nil {
		return nil
	}
	res := make([]MCPToolDefinition, 0, len(m.Tools))
	for _, t := range m.Tools {
		res = append(res, ToolToMCP(&t))
	}
	return res
}

// ToolToMCP converts a single UTCP tool into an MCP tool definition.
func ToolToMCP(t *Tool) MCPToolDefinition {
	if t == nil {
		return MCPToolDefinition{}
	}
	return MCPToolDefinition{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.Inputs,
	}
}

// ParseManualJSON deserializes and validates a UTCP manual JSON payload.
func ParseManualJSON(data []byte) (*Manual, error) {
	var m Manual
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal utcp manual: %w", err)
	}
	if err := ValidateManual(&m); err != nil {
		return nil, err
	}
	return &m, nil
}
