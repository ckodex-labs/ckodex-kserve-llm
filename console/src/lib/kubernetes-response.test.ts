import assert from "node:assert/strict";
import test from "node:test";

import { kubernetesResponseBody } from "./kubernetes-response.ts";

test("accepts current direct Kubernetes client responses", () => {
    const list = { apiVersion: "serving.ckodex.com/v1", items: [{ metadata: { name: "model-a" } }] };
    assert.equal(kubernetesResponseBody(list), list);
});

test("retains compatibility with wrapped Kubernetes client responses", () => {
    const list = { apiVersion: "serving.ckodex.com/v1", items: [{ metadata: { name: "model-a" } }] };
    assert.equal(kubernetesResponseBody({ body: list }), list);
});
