# Author and bind UTCP tools

This guide explains how to author Universal Tool Calling Protocol (UTCP v1.0.1) tool manuals, package them as immutable OCI artifacts via `AIPack`, and bind them to autonomous agents using `Agent` and `SkillRegistry`.

## Overview

The Universal Tool Calling Protocol (UTCP v1.0.1) allows autonomous language agents to discover and invoke tools over standard HTTP, gRPC, and CLI transports without requiring custom proxy middleware.

The operator provides pure-domain UTCP serialization, validation, and conversion in `internal/utcp/`, supporting direct interoperability with OpenAI tool calling and Model Context Protocol (MCP) schemas.

## Prerequisites

- Operator version v0.20.0 installed in your cluster.
- The `oras` CLI installed for packaging OCI artifacts.
- The `cosign` CLI installed for signing artifacts.
- An OCI container registry accessible by your cluster.

## Step 1: Author a UTCP v1.0.1 manual

Create a JSON file named `document-tools.json` defining tool signatures, input schemas, and execution endpoints:

```json
{
  "utcp_version": "1.0.1",
  "manual_version": "1.0.0",
  "info": {
    "title": "Document Analysis Toolset",
    "description": "Tools for parsing invoices and extracting tables",
    "version": "1.0.0"
  },
  "tools": [
    {
      "name": "parse_invoice",
      "description": "Parse an invoice image into structured JSON fields",
      "inputs": {
        "type": "object",
        "properties": {
          "document_url": {
            "type": "string",
            "format": "uri"
          },
          "extract_tables": {
            "type": "boolean",
            "default": true
          }
        },
        "required": ["document_url"]
      },
      "tool_call_template": {
        "call_template_type": "http",
        "url": "http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/v1/chat/completions",
        "http_method": "POST"
      }
    }
  ]
}
```

Verify your JSON manual against the schema using `jq`:

```bash
jq . document-tools.json > /dev/null && echo "Valid JSON"
```

## Step 2: Package the manual into an immutable AIPack

Push the manual to your OCI registry as an immutable `UTCPManual` artifact (kind code `A15`):

```bash
REGISTRY="registry.corp.internal"
REPO="aipack/document-tools"
TAG="v1.0.0"

oras push ${REGISTRY}/${REPO}:${TAG} \
  --config /dev/null:application/vnd.ai.utcpmanual.v1+json \
  ./document-tools.json:application/vnd.ai.utcpmanual.v1+json
```

Capture the digest of the pushed artifact:

```bash
DIGEST=$(oras discover -o json ${REGISTRY}/${REPO}:${TAG} | jq -r '.manifests[0].digest // empty')
if [ -z "$DIGEST" ]; then
  DIGEST=$(skopeo inspect docker://${REGISTRY}/${REPO}:${TAG} | jq -r .Digest)
fi
echo "Pushed artifact digest: ${DIGEST}"
```

## Step 3: Sign the artifact with Cosign

Sign the artifact using your Cosign private key:

```bash
cosign sign --key /keys/cosign.key ${REGISTRY}/${REPO}@${DIGEST}
```

## Step 4: Declare the AIPack custom resource

Create a manifest named `aipack-document-tools.yaml` with mandatory digest pinning:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: AIPack
metadata:
  name: aipack-document-tools
  namespace: default
spec:
  kind: UTCPManual
  family: capability
  source:
    ref: registry.corp.internal/aipack/document-tools@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    mediaType: application/vnd.ai.utcpmanual.v1+json
  utcpManual:
    utcpVersion: "1.0.1"
    manualVersion: "1.0.0"
    endpoint: http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/utcp
```

Replace the sample digest with your real artifact digest, then apply the manifest:

```bash
kubectl apply -f aipack-document-tools.yaml
```

Check the status conditions:

```bash
kubectl get aipack aipack-document-tools -o yaml
```

The conditions list should show `Admitted: True` and `IntegrityVerified: True`.

## Step 5: Bind tools to an autonomous agent

Declare an `Agent` custom resource (`serving.ckodex.com/v1alpha2`) referencing the base model and the tool manual:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: Agent
metadata:
  name: document-analyst
  namespace: default
spec:
  identity:
    name: Document Analyst Agent
    version: 1.0.0
  modelRef: quickstart-llm
  utcpManuals:
    - name: doc-tools
      packRef: aipack-document-tools
  maxTokens: 4096
```

Apply the agent manifest:

```bash
kubectl apply -f agent-document-analyst.yaml
```

Verify that the operator admits the agent and validates its bindings:

```bash
kubectl describe agent document-analyst
```

## Step 6: Security rules for tool templates

Before deploying tool manuals to production environments, verify the following security controls:

1. **Absolute CLI binaries**: If using `call_template_type: cli`, do not pass shell interpreters (`/bin/sh`, `/bin/bash`). Specify absolute, immutable binary paths with explicit argument arrays.
2. **Cluster internal URLs**: HTTP templates must use explicit internal Kubernetes DNS names (`.svc.cluster.local`) rather than raw IP addresses or untrusted external hostnames.
3. **Write side-effect declarations**: Tools that alter external state must set `idempotent: false` and declare `sideEffects: write` in their metadata.

## Next steps

- [Deploy multimodal foundation models](deploy-multimodal-ocr-sam3.md)
- [Consult AIPack kinds in the CRD reference](../reference/crds.md)
- [Review security architecture](../explanation/security-architecture.md)
