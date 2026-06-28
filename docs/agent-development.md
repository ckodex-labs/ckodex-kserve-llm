# Experimental Agent Metadata

## Current Capability

`Agent` and `SkillRegistry` have stable v1 API schemas, but their controllers
are still experimental product features. Their
controllers are disabled by default and are enabled with:

```text
CKODEX_FEATURE_ENABLE_EXPERIMENTAL_AGENTS=true
```

Today the controllers:

- validate that an Agent references an existing, ready
  `LLMInferenceService`;
- validate that referenced skill names exist in a `SkillRegistry`;
- validate required skill metadata and duplicate names;
- publish readiness conditions and entry counts.

They do not deploy an agent runtime, intercept model tool calls, invoke skill
endpoints, or expose an agent-specific inference gateway.

## Register Skill Metadata

```yaml
apiVersion: serving.ckodex.com/v1
kind: SkillRegistry
metadata:
  name: research-tools
  namespace: inference
spec:
  entries:
    - name: search-papers
      version: 1.0.0
      description: Search an internal paper index
      endpoint: http://paper-search.inference.svc.cluster.local/query
      inputSchema: |
        {
          "type": "object",
          "properties": {
            "query": {"type": "string"}
          },
          "required": ["query"]
        }
```

## Bind Agent Metadata

```yaml
apiVersion: serving.ckodex.com/v1
kind: Agent
metadata:
  name: research-assistant
  namespace: inference
spec:
  identity:
    name: Research Assistant
    description: Metadata binding for a research model and paper search skill
    version: 0.1.0
  modelRef: llama-3-8b
  maxTokens: 4096
  skills:
    - registryRef: research-tools
      skillName: search-papers
      version: 1.0.0
```

Apply and inspect:

```bash
kubectl apply -f skill-registry.yaml
kubectl apply -f agent.yaml
kubectl get skillregistry,agent -n inference
kubectl describe agent research-assistant -n inference
```

An `Agent` reporting ready means its metadata references are valid. It does not
mean an agent execution service is running.

## Building an Execution Runtime

An external agent runtime may consume these resources as configuration. That
runtime must independently implement:

- request authentication and authorization;
- model invocation;
- tool-call parsing and execution;
- endpoint allowlisting and network policy;
- retries, timeouts, and output validation;
- audit and trace correlation.

Do not route untrusted model-generated arguments directly to a skill endpoint.

## Source of Truth

- API: `api/v1alpha2/agent_types.go`
- Controllers: `internal/controller/agent_controller.go` and
  `internal/controller/skillregistry_controller.go`
- Feature gate: `internal/config/operator_config.go`
