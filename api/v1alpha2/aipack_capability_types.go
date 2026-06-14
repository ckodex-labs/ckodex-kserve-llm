/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha2

// SkillSpec describes an A4 Skill artifact per AIPACK-SPEC v0.1.1 §3.3.
type SkillSpec struct {
	// Runtime declares the execution environment.
	// Examples: "python3.12", "node20", "wasm", "native"
	Runtime string `json:"runtime"`

	// EntryPoint is the entry point within the skill bundle.
	// +optional
	EntryPoint string `json:"entryPoint,omitempty"`

	// CapabilityDeclaration enumerates the capabilities this skill exposes.
	// Each entry corresponds to a CapabilitySpec URN.
	// +optional
	CapabilityDeclaration []string `json:"capabilityDeclaration,omitempty"`

	// SandboxPolicy declares the sandbox policy mode.
	// Values: "strict", "standard", "permissive"
	// Backed by attestation urn:skill:safety-review:v1.
	// +optional
	// +kubebuilder:validation:Enum=strict;standard;permissive
	SandboxPolicy string `json:"sandboxPolicy,omitempty"`

	// MaxConcurrency is the maximum concurrent invocations the skill supports.
	// +optional
	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`

	// TimeoutSeconds is the per-invocation timeout.
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// ToolSpec describes an A5 Tool artifact per AIPACK-SPEC v0.1.1 §3.3.
type ToolSpec struct {
	// Schema is the JSON Schema URI or inline schema for this tool's input/output.
	// Backed by attestation urn:tool:schema-validation:v1.
	// +optional
	Schema string `json:"schema,omitempty"`

	// Protocol declares the tool invocation protocol.
	// Values: "function-call", "http", "grpc", "mcp"
	// +optional
	// +kubebuilder:validation:Enum=function-call;http;grpc;mcp
	Protocol string `json:"protocol,omitempty"`

	// Idempotent declares whether the tool is safe to retry.
	// +optional
	Idempotent *bool `json:"idempotent,omitempty"`

	// SideEffects declares whether this tool has external side effects.
	// Values: "none", "read-only", "write"
	// +optional
	// +kubebuilder:validation:Enum=none;read-only;write
	SideEffects string `json:"sideEffects,omitempty"`

	// MaxResponseBytes is the maximum response payload size in bytes.
	// +optional
	MaxResponseBytes *int64 `json:"maxResponseBytes,omitempty"`
}

// MCPServerSpec describes an A6 MCPServer artifact per AIPACK-SPEC v0.1.1 §3.3.
type MCPServerSpec struct {
	// ToolList is the list of tool names exposed by this MCP server.
	// Backed by attestation urn:mcp:tool-list:v1.
	// +optional
	ToolList []string `json:"toolList,omitempty"`

	// SandboxPolicy declares the sandbox enforcement mode for the MCP server.
	// Values: "strict", "standard", "permissive"
	// Backed by attestation urn:mcp:sandbox-policy:v1.
	// +optional
	// +kubebuilder:validation:Enum=strict;standard;permissive
	SandboxPolicy string `json:"sandboxPolicy,omitempty"`

	// Transport declares the MCP transport protocol.
	// Values: "stdio", "http+sse", "websocket"
	// +optional
	// +kubebuilder:validation:Enum=stdio;http+sse;websocket
	Transport string `json:"transport,omitempty"`

	// MaxConcurrency is the maximum number of concurrent sessions.
	// +optional
	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`
}

// WorkflowSpec describes an A13 Workflow artifact per AIPACK-SPEC v0.1.1 §3.3.
type WorkflowSpec struct {
	// Engine declares the workflow orchestration engine.
	// Examples: "dapr", "temporal", "argo", "custom"
	// +optional
	Engine string `json:"engine,omitempty"`

	// Entrypoint is the workflow entrypoint name or ID.
	// +optional
	Entrypoint string `json:"entrypoint,omitempty"`

	// Deterministic declares whether this workflow produces deterministic outputs.
	// +optional
	Deterministic *bool `json:"deterministic,omitempty"`

	// MaxDurationSeconds is the maximum allowed workflow run duration.
	// +optional
	MaxDurationSeconds *int64 `json:"maxDurationSeconds,omitempty"`

	// StepCount is the declared step count in the workflow graph.
	// +optional
	StepCount *int32 `json:"stepCount,omitempty"`
}
