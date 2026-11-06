# UTCP tools operations runbook

This operational runbook details procedures for authoring, publishing, binding, and troubleshooting Universal Tool Calling Protocol (UTCP v1.0.1) tools across the CKodex platform.

## Publishing tool manuals

Tools are declared in standardized JSON manuals conforming to the UTCP v1.0.1 schema. Manuals can be hosted via an HTTP catalog service or packaged into an immutable OCI artifact via the `AIPack` CRD.

### Option 1: Publishing via local HTTP catalog

1. Create the manual JSON document (for example, `vision-tools.json`):
   ```json
   {
     "utcp_version": "1.0.1",
     "manual_version": "1.0.0",
     "info": {
       "title": "Document Analysis Tools",
       "version": "1.0.0"
     },
     "tools": [
       {
         "name": "unlimited_ocr_parse",
         "description": "Parse multi-page documents with Baidu Unlimited-OCR-2",
         "inputs": {
           "type": "object",
           "properties": {
             "document_url": { "type": "string" },
             "pages": { "type": "array", "items": { "type": "integer" } }
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
2. Upload the manual to your cluster catalog service (such as SeaweedFS, MinIO, or an internal static file server):
   ```bash
   curl -X PUT -T vision-tools.json http://utcp-catalog.default.svc.cluster.local/utcp/vision-tools.json
   ```
3. Verify manual retrieval and schema validity:
   ```bash
   curl -s http://utcp-catalog.default.svc.cluster.local/utcp/vision-tools.json | jq .
   ```

### Option 2: Publishing as an immutable AIPack

1. Package the manual into an OCI container image or artifact:
   ```bash
   oras push registry.corp.internal/aipack/unlimited-ocr-utcp:v1.0.0 \
     --config /dev/null:application/vnd.ai.utcpmanual.v1+json \
     ./vision-tools.json:application/vnd.ai.utcpmanual.v1+json
   ```
2. Sign the artifact with Cosign:
   ```bash
   cosign sign --key /keys/cosign.key registry.corp.internal/aipack/unlimited-ocr-utcp:v1.0.0
   ```
3. Declare the `AIPack` custom resource in Kubernetes:
   ```yaml
   apiVersion: serving.ckodex.com/v1alpha2
   kind: AIPack
   metadata:
     name: aipack-unlimited-ocr-utcp
     namespace: default
   spec:
     kind: UTCPManual
     family: capability
     source:
       ref: registry.corp.internal/aipack/unlimited-ocr-utcp@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
       mediaType: application/vnd.ai.utcpmanual.v1+json
     utcpManual:
       utcpVersion: "1.0.1"
       manualVersion: "1.0.0"
       endpoint: http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/utcp
   ```
4. Apply and verify AIPack status:
   ```bash
   kubectl apply -f aipack-unlimited-ocr.yaml
   kubectl get aipack aipack-unlimited-ocr-utcp -o jsonpath='{.status.conditions}'
   ```

## Binding tools to autonomous agents

Bind published manuals to an autonomous agent using the `Agent` CRD:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: Agent
metadata:
  name: document-analyst
  namespace: default
spec:
  identity:
    name: Document Analyst
    version: 1.0.0
  modelRef: kimi-k31-specimen
  utcpManuals:
    - name: vision-tools
      endpoint: http://utcp-catalog.default.svc.cluster.local/utcp/vision-tools.json
    - name: ocr-manual
      packRef: aipack-unlimited-ocr-utcp
  maxTokens: 4096
```

Apply the agent manifest:

```bash
kubectl apply -f agent-document-analyst.yaml
```

The operator validates manual references and checks that all target services exist and expose active endpoints.

## Troubleshooting UTCP execution

### Error 1: Schema validation error before dispatch
**Symptom**: Agent logs `UTCPValidationError: input payload fails schema validation`.
**Cause**: The LLM produced arguments missing required properties or containing wrong types.
**Diagnosis**:
`Agent` is a declarative configuration and binding resource rather than an operator-managed Deployment. Inspect the client application pod running the agent runtime, or inspect controller manager logs for reconciliation and validation events:
```bash
# Inspect client application pod logs
kubectl logs deploy/<client-app> -c agent | grep UTCPValidationError

# Or inspect operator controller manager logs
kubectl logs -n ckodex-system -l control-plane=controller-manager -c manager | grep document-analyst
```
**Remediation**:
- Verify the tool `inputs` JSON schema in the manual.
- Check the specimen model tool call parser configuration (for example, `--tool-call-parser=hermes`).
- Add few-shot tool calling examples to the agent system prompt.

### Error 2: Network connection refused or timeout
**Symptom**: Agent logs `UTCPDispatchError: dial tcp: connection refused` or context deadline exceeded.
**Cause**: The endpoint specified in `tool_call_template` is unreachable, port is misconfigured, or Kubernetes NetworkPolicy blocks traffic.
**Diagnosis**:
1. Test endpoint connectivity from client application pods or a diagnostic pod:
   ```bash
   kubectl exec -it deploy/<client-app> -c agent -- \
     curl -v http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/health
   ```
2. Inspect target service endpoints:
   ```bash
   kubectl get endpoints unlimited-ocr-2 -n ckodex-inference
   ```
3. Check NetworkPolicy rules in both namespaces.

### Error 3: Remote tool returns non-2xx status code
**Symptom**: Tool execution receipt reports `HTTP 500` or `HTTP 400` from target API.
**Cause**: Target model or service failed during processing (for example, out-of-memory or invalid input image).
**Diagnosis**:
Inspect the target workload logs:
```bash
kubectl logs deploy/unlimited-ocr-2 -c vllm --tail=100
```
**Remediation**: Check image payload size and GPU VRAM capacity on the target node.

## Security auditing of tool call templates

Before promoting a UTCP manual to production, conduct a security audit against these invariants:
1. **Restrict CLI templates**: `call_template_type: cli` must never execute arbitrary shell interpreters (`/bin/sh`, `/bin/bash`). Command paths must be absolute, immutable binaries.
2. **Validate URL schemes**: HTTP templates must use explicit internal DNS names (`.svc.cluster.local`). External URLs must be reviewed and restricted by egress network policies.
3. **Audit side effects**: Ensure tools modifying persistent state declare `idempotent: false` and `sideEffects: write` in their AIPack manifests.
