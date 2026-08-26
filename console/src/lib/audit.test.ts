import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { auditEventGlyph, auditEventSummary, parseAuditEvent } from "./audit.ts";

const [runtimeRecord] = readFileSync(
    new URL("../../test/fixtures/audit-runtime.jsonl", import.meta.url),
    "utf8",
).trim().split("\n");

test("parses the operator AuditEvent JSON contract", () => {
    const event = parseAuditEvent(runtimeRecord);

    assert.deepEqual(event, {
        action: "PolicyViolation",
        resource: "LLMInferenceService/default/model-a",
        actor: "opa-gatekeeper",
        outcome: "Denied",
        timestamp: "2026-08-02T14:31:05Z",
        details: {
            policy: "approved-model-source",
            violation: "registry not allowed",
        },
        reason: "Model source did not satisfy policy.",
        execId: "urn:ckodex:exec:01",
        execKind: "governance",
        reproducibilityClass: "explanatory",
    });
});

test("rejects the retired console-only mock schema", () => {
    const legacyRecord = JSON.stringify({
        timestamp: "2026-08-02T14:31:05Z",
        level: "INFO",
        event: "TestEvent",
        message: "Console initialized",
        resource: "console",
        action: "Start",
    });

    assert.equal(parseAuditEvent(legacyRecord), null);
});

test("rejects non-string structured details", () => {
    const invalidRecord = JSON.stringify({
        action: "TokenConsumed",
        resource: "model-a",
        actor: "subject-a",
        outcome: "Success",
        timestamp: "2026-08-02T14:31:05Z",
        details: { total_tokens: 120 },
    });

    assert.equal(parseAuditEvent(invalidRecord), null);
});

test("derives proof-literate display text without inventing severity", () => {
    const event = parseAuditEvent(runtimeRecord);
    assert.ok(event);
    assert.equal(auditEventGlyph(event.outcome), "⊭");
    assert.equal(auditEventSummary(event), "Model source did not satisfy policy.");
});
