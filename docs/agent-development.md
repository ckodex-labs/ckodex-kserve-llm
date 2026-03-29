# Agent Development Guide

This guide describes how to build and deploy AI agents that leverage the LLM inference backends and specialized function-calling tools within the CKodex cluster.

## Overview
Agents are higher-level abstractions that bind a specific **LLMInferenceService** (the "brain") to a set of **SkillRegistries** (the "tools").

## Step 1: Connecting an Agent to a Model

Use the `Agent` CRD to define the agent's identity and its underlying model backend.

```yaml
apiVersion: serving.ckodex.io/v1alpha2
kind: Agent
metadata:
  name: customer-support-agent
spec:
  identity:
    name: "SupportBot v2"
    description: "Handles L1 support tickets via RAG and tools"
  modelRef: "llama-3-8b" # Reference to an LLMInferenceService
  maxTokens: 4096
```

## Step 2: Defining & Registering Skills

Skills are reusable tool definitions (functions) that an agent can invoke during its inference cycle. These are managed via `SkillRegistry`.

```yaml
apiVersion: serving.ckodex.io/v1alpha2
kind: SkillRegistry
metadata:
  name: platform-tools
spec:
  entries:
    - name: "get_ticket_status"
      version: "1.0.0"
      description: "Retrieves the status of a support ticket"
      endpoint: "http://ticketing-system.internal.svc/v1/status"
      inputSchema: |
        {
          "type": "object",
          "properties": {
            "ticket_id": { "type": "string" }
          },
          "required": ["ticket_id"]
        }
```

## Step 3: Binding Skills to the Agent

You can bind specific skills to your agent using `SkillRef`.

```yaml
apiVersion: serving.ckodex.io/v1alpha2
kind: Agent
metadata:
  name: customer-support-agent
spec:
  # ... identity and modelRef ...
  skills:
    - registryRef: "platform-tools"
      skillName: "get_ticket_status"
```

## Step 4: Invocation & Tool Use

Clients interact with agents via the standard OpenAI-compatible `/v1/chat/completions` endpoint.

- **System Prompt**: The operator automatically injects the agent's identity and tool descriptions into the system prompt of the inference request.
- **Function Calling**: When the model emits a tool call (e.g., `get_ticket_status`), the agent-sidecar (EPP) intercepts the call, executes the tool against the registered `endpoint`, and returns the result to the model for final answer generation.

### Example Request (via Python SDK):
```python
import openai

client = openai.OpenAI(base_url="http://agent-gateway.internal.svc/v1")

response = client.chat.completions.create(
    model="customer-support-agent",
    messages=[{"role": "user", "content": "What is the status of ticket #INC-99?"}]
)

print(response.choices[0].message.content)
```

## Best Practices
- **Security**: Always use **SPIRE** sidecars for agent workloads to ensure mutual TLS when communicating with SkillRegistry endpoints.
- **Versioning**: Pin your agent's skills to specific versions (e.g., `version: "1.2.0"`) to avoid breaking changes during automated registry updates.
- **Monitoring**: Monitor the `agent_tool_invocation_latency` and `agent_token_usage` metrics via Prometheus to track operational costs and performance.
