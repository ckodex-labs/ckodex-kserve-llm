/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package utcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleValidManualJSON() string {
	return `{
		"utcp_version": "1.0.1",
		"manual_version": "1.0.0",
		"info": {
			"title": "Document & Search Services",
			"description": "Native tools for document OCR, search, and concept segmentation",
			"version": "1.0.0"
		},
		"tools": [
			{
				"name": "unlimited_ocr_parse",
				"description": "Parse long documents with Baidu Unlimited-OCR-2",
				"inputs": {
					"type": "object",
					"properties": {
						"document_url": {"type": "string"},
						"pages": {"type": "array", "items": {"type": "integer"}}
					},
					"required": ["document_url"]
				},
				"tool_call_template": {
					"call_template_type": "http",
					"url": "https://ocr.corp.internal/v1/parse",
					"http_method": "POST"
				}
			},
			{
				"name": "sam3_segment_concept",
				"description": "Segment concept prompt via Meta SAM 3",
				"inputs": {
					"type": "object",
					"properties": {
						"image_url": {"type": "string"},
						"concept": {"type": "string"}
					},
					"required": ["image_url", "concept"]
				},
				"tool_call_template": {
					"call_template_type": "grpc",
					"endpoint": "sam3.corp.internal:9000",
					"service": "sam3.SegmentationService",
					"method": "SegmentConcept"
				}
			},
			{
				"name": "local_document_indexer",
				"description": "CLI utility to index parsed documents locally",
				"inputs": {
					"type": "object",
					"properties": {
						"file_path": {"type": "string"}
					},
					"required": ["file_path"]
				},
				"tool_call_template": {
					"call_template_type": "cli",
					"command": "docindex",
					"args": ["--scan"]
				}
			}
		]
	}`
}

func TestParseManualJSON_Valid(t *testing.T) {
	data := []byte(sampleValidManualJSON())
	m, err := ParseManualJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "1.0.1", m.UTCPVersion)
	assert.Equal(t, "1.0.0", m.ManualVersion)
	assert.Equal(t, "Document & Search Services", m.Info.Title)
	require.Len(t, m.Tools, 3)

	assert.Equal(t, "unlimited_ocr_parse", m.Tools[0].Name)
	assert.Equal(t, TemplateTypeHTTP, m.Tools[0].ToolCallTemplate.CallTemplateType)
	assert.Equal(t, "POST", m.Tools[0].ToolCallTemplate.HTTPMethod)

	assert.Equal(t, "sam3_segment_concept", m.Tools[1].Name)
	assert.Equal(t, TemplateTypeGRPC, m.Tools[1].ToolCallTemplate.CallTemplateType)
	assert.Equal(t, "sam3.SegmentationService", m.Tools[1].ToolCallTemplate.Service)

	assert.Equal(t, "local_document_indexer", m.Tools[2].Name)
	assert.Equal(t, TemplateTypeCLI, m.Tools[2].ToolCallTemplate.CallTemplateType)
	assert.Equal(t, "docindex", m.Tools[2].ToolCallTemplate.Command)
}

func TestToOpenAITools(t *testing.T) {
	data := []byte(sampleValidManualJSON())
	m, err := ParseManualJSON(data)
	require.NoError(t, err)

	openAITools := ToOpenAITools(m)
	require.Len(t, openAITools, 3)

	assert.Equal(t, "function", openAITools[0].Type)
	assert.Equal(t, "unlimited_ocr_parse", openAITools[0].Function.Name)
	assert.Equal(t, "Parse long documents with Baidu Unlimited-OCR-2", openAITools[0].Function.Description)
	assert.NotNil(t, openAITools[0].Function.Parameters)
}

func TestToMCPTools(t *testing.T) {
	data := []byte(sampleValidManualJSON())
	m, err := ParseManualJSON(data)
	require.NoError(t, err)

	mcpTools := ToMCPTools(m)
	require.Len(t, mcpTools, 3)

	assert.Equal(t, "sam3_segment_concept", mcpTools[1].Name)
	assert.Equal(t, "Segment concept prompt via Meta SAM 3", mcpTools[1].Description)
	assert.NotNil(t, mcpTools[1].InputSchema)
}

func TestValidateManual_Failures(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Manual)
		wantErr string
	}{
		{
			name: "missing utcp_version",
			mutate: func(m *Manual) {
				m.UTCPVersion = ""
			},
			wantErr: "utcp_version is required",
		},
		{
			name: "missing manual_version",
			mutate: func(m *Manual) {
				m.ManualVersion = ""
			},
			wantErr: "manual_version is required",
		},
		{
			name: "missing title",
			mutate: func(m *Manual) {
				m.Info.Title = ""
			},
			wantErr: "info.title is required",
		},
		{
			name: "empty tool name",
			mutate: func(m *Manual) {
				m.Tools[0].Name = ""
			},
			wantErr: "tool name is required",
		},
		{
			name: "duplicate tool name",
			mutate: func(m *Manual) {
				m.Tools[1].Name = m.Tools[0].Name
			},
			wantErr: "duplicate tool name",
		},
		{
			name: "missing inputs schema",
			mutate: func(m *Manual) {
				m.Tools[0].Inputs = nil
			},
			wantErr: "tool inputs schema is required",
		},
		{
			name: "missing tool call template",
			mutate: func(m *Manual) {
				m.Tools[0].ToolCallTemplate = nil
			},
			wantErr: "tool_call_template is required",
		},
		{
			name: "invalid http method",
			mutate: func(m *Manual) {
				m.Tools[0].ToolCallTemplate.HTTPMethod = "INVALID"
			},
			wantErr: "unsupported http_method 'INVALID'",
		},
		{
			name: "missing http url",
			mutate: func(m *Manual) {
				m.Tools[0].ToolCallTemplate.URL = ""
			},
			wantErr: "url is required for http template",
		},
		{
			name: "missing grpc endpoint",
			mutate: func(m *Manual) {
				m.Tools[1].ToolCallTemplate.Endpoint = ""
			},
			wantErr: "endpoint is required for grpc template",
		},
		{
			name: "missing cli command",
			mutate: func(m *Manual) {
				m.Tools[2].ToolCallTemplate.Command = ""
			},
			wantErr: "command is required for cli template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseManualJSON([]byte(sampleValidManualJSON()))
			require.NoError(t, err)
			tt.mutate(m)
			err = ValidateManual(m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
