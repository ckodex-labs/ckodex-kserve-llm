# Universal Tool Calling Protocol (UTCP) Integration Guide

The **Universal Tool Calling Protocol (UTCP)** is an open standard designed to enable AI agents to discover, inspect, and invoke tools directly over their native protocols (HTTP, gRPC, CLI, WebSockets, SSE) without requiring intermediate wrapper proxy servers.

---

## 1. Core Architectural Philosophy

Traditional tool-calling integration models (such as naive MCP deployments) often introduce a **"wrapper tax"**: developers must build, containerize, host, and maintain dedicated proxy/adapter servers for every tool, even when a perfectly functional API already exists.

```text
Traditional MCP Proxy Pattern:
LLM Agent → MCP Client → [Middleman MCP Server] → Native Tool / REST / gRPC API
                            (Wrapper Tax: extra hops, auth duplication, ops overhead)

UTCP Direct Invocation Pattern:
LLM Agent → UTCP Manual (/utcp) → Direct Call to Native Endpoint (HTTP/gRPC/CLI)
```

The UTCP philosophy:
> *"If humans or microservices can interact with an API, AI should be able to do the same over the native protocol with no changes to the target API and identical zero-trust security."*

---

## 2. Multi-Language SDK & Ecosystem Alignment

The operator provides full architectural and schema alignment with the official UTCP repositories:

- **Specification**: [`universal-tool-calling-protocol/utcp-specification`](https://github.com/universal-tool-calling-protocol/utcp-specification) (v1.0.1 schema).
- **MCP Bridge**: [`universal-tool-calling-protocol/utcp-mcp`](https://github.com/universal-tool-calling-protocol/utcp-mcp) (allows MCP clients to discover and call UTCP manuals via `register_manual`, `call_tool`, and `search_tools`).
- **Language SDKs**:
  - Pure Go domain library: `internal/utcp` (compatible with [`go-utcp`](https://github.com/universal-tool-calling-protocol/go-utcp)).
  - Rust SDK: [`rs-utcp`](https://github.com/universal-tool-calling-protocol/rs-utcp).
  - TypeScript SDK: [`typescript-utcp`](https://github.com/universal-tool-calling-protocol/typescript-utcp).
  - Python SDK: [`python-utcp`](https://github.com/universal-tool-calling-protocol/python-utcp).

---

## 3. The UTCP Manual Specification

A UTCP tool provider exposes a standardized JSON manifest called a **Manual**, conventionally served at `GET /utcp` or `/.well-known/utcp.json`:

```json
{
  "utcp_version": "1.0.1",
  "manual_version": "1.0.0",
  "info": {
    "title": "Computer Vision & Document Services",
    "description": "Native tools for multi-page OCR and concept segmentation",
    "version": "1.0.0"
  },
  "tools": [
    {
      "name": "unlimited_ocr_parse",
      "description": "Parse long-horizon documents with Baidu Unlimited-OCR-2",
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
        "url": "http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/v1/chat/completions",
        "http_method": "POST"
      }
    },
    {
      "name": "sam3_segment_concept",
      "description": "Promptable concept segmentation with Meta SAM 3",
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
        "endpoint": "sam3.ckodex-inference.svc.cluster.local:9000",
        "service": "sam3.SegmentationService",
        "method": "SegmentConcept"
      }
    }
  ]
}
```

---

## 4. Workload Integrations

### 4.1 Baidu Unlimited-OCR-2 (`baidu/Unlimited-OCR-2`)
- **Task**: `ocr`
- **Architecture**: 3B Vision-Language document parser equipped with Sparse Reference Sliding Window Attention (SR-SWA).
- **Use Case**: One-shot parsing of multi-page documents (100–300+ pages) into markdown, tables, and formulas.
- **Manifest**: [`config/samples/multimodalinferenceservice_unlimited_ocr.yaml`](../config/samples/multimodalinferenceservice_unlimited_ocr.yaml).

### 4.2 Meta SAM 3 (`facebook/sam3`)
- **Task**: `segmentation`
- **Architecture**: Segment Anything Model 3 foundation model for Promptable Concept Segmentation (PCS).
- **Use Case**: Zero-shot object identification, segmentation masks, and tracking using natural language concepts or visual exemplars.
- **Manifest**: [`config/samples/multimodalinferenceservice_sam3_segmentation.yaml`](../config/samples/multimodalinferenceservice_sam3_segmentation.yaml).

### 4.3 Primary Specimen Model: `tiny-random/kimi-k3.1-pro`
- **Architecture**: Moonshot AI Kimi K3.1 Pro MoE (Kimi Delta Attention / KDA, Gated-DSA).
- **Role**: Primary specimen model for testing tool-calling execution, reasoning loops, and prompt parsers without requiring multi-GPU clusters.
- **Manifest**: [`config/samples/llminferenceservice_kimi_k31_specimen.yaml`](../config/samples/llminferenceservice_kimi_k31_specimen.yaml).
- **Automated Preset**: Recognizes `tiny-random/kimi-k3.1-pro` with `--trust-remote-code`, `--enforce-eager`, and `--tool-call-parser hermes`.

---

## 5. Declarative Agent UTCP Configuration

Agents define UTCP manuals via the `Agent` CRD (`serving.ckodex.com/v1alpha2`):

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: Agent
metadata:
  name: vision-analyst
  namespace: default
spec:
  identity:
    name: Vision Analyst
    version: 1.0.0
  modelRef: kimi-k31-specimen
  utcpManuals:
    - name: vision-tools
      endpoint: http://utcp-catalog.default.svc.cluster.local/utcp
  tools:
    - name: unlimited_ocr_parse
      protocol: utcp
      endpoint: http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/v1/chat/completions
      inputSchema: '{"type":"object","properties":{"document_url":{"type":"string"}},"required":["document_url"]}'
```

---

## 6. Bridge with MCP (`utcp-mcp`)

When an existing environment utilizes Model Context Protocol (MCP) clients (such as Claude Desktop or Cursor), the operator supports `utcp-mcp` bridging via `internal/utcp.ToMCPTools(manual)`. This exposes all native UTCP tools to the MCP client seamlessly while keeping execution native.
