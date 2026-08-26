import assert from "node:assert/strict";
import test from "node:test";

import {
    capabilityDecision,
    consoleCapabilitySpecs,
    parseSpiffeId,
    parseSpireRegistration,
} from "./identity.ts";

const registration = {
    metadata: {
        name: "ckodex-spire-entry-tenant-a-model-a",
        namespace: "spire",
        uid: "registration-01",
        labels: { "ckodex.com/tenant-id": "tenant-a" },
        annotations: {
            "ckodex.com/source-namespace": "tenant-a",
            "ckodex.com/source-service": "model-a",
            "ckodex.com/spiffe-id": "spiffe://ckodex.com/ns/tenant-a/sa/model-a/model/model-a",
        },
    },
    data: {
        "entry.json": JSON.stringify({
            spiffeId: "spiffe://ckodex.com/ns/tenant-a/sa/model-a/model/model-a",
            selectors: ["k8s:ns:tenant-a", "k8s:sa:model-a"],
            ttl: 3600,
            dnsSans: ["model-a.tenant-a.svc.cluster.local"],
        }),
    },
};

test("parses an operator SPIRE registration without asserting SVID validity", () => {
    assert.deepEqual(parseSpireRegistration(registration), {
        id: "registration-01",
        configMapName: "ckodex-spire-entry-tenant-a-model-a",
        registrationNamespace: "spire",
        spiffeId: "spiffe://ckodex.com/ns/tenant-a/sa/model-a/model/model-a",
        trustDomain: "ckodex.com",
        parentId: null,
        selectors: ["k8s:ns:tenant-a", "k8s:sa:model-a"],
        ttlSeconds: 3600,
        dnsSans: ["model-a.tenant-a.svc.cluster.local"],
        sourceNamespace: "tenant-a",
        sourceService: "model-a",
        tenantId: "tenant-a",
    });
});

test("rejects malformed, credential-bearing, and annotation-mismatched identities", () => {
    assert.equal(parseSpiffeId("https://ckodex.com/workload"), null);
    assert.equal(parseSpiffeId("spiffe://user:secret@ckodex.com/workload"), null);
    assert.equal(parseSpireRegistration({ ...registration, data: { "entry.json": "{" } }), null);
    assert.equal(parseSpireRegistration({
        ...registration,
        metadata: {
            ...registration.metadata,
            annotations: { ...registration.metadata.annotations, "ckodex.com/spiffe-id": "spiffe://ckodex.com/different" },
        },
    }), null);
});

test("keeps capability decisions distinct from transport failures", () => {
    assert.equal(capabilityDecision(true, false), "allowed");
    assert.equal(capabilityDecision(false, true), "denied");
    assert.equal(capabilityDecision(false, false), "no-opinion");
    assert.equal(consoleCapabilitySpecs.filter((item) => item.effect === "mutate").length, 2);
});
