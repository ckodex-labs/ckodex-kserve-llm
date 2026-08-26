import assert from "node:assert/strict";
import test from "node:test";

import { resolveAiGatewayBaseUrl } from "./ai-gateway-url.ts";

test("requires HTTPS for remote AI gateways unless insecure transport is explicit", () => {
    assert.equal(resolveAiGatewayBaseUrl("https://gateway.example.com/v1/", false)?.toString(), "https://gateway.example.com/v1");
    assert.equal(resolveAiGatewayBaseUrl("http://127.0.0.1:3120/v1", false)?.toString(), "http://127.0.0.1:3120/v1");
    assert.equal(resolveAiGatewayBaseUrl("http://api.tprime.vlans.ca/v1", false), null);
    assert.equal(resolveAiGatewayBaseUrl("http://api.tprime.vlans.ca/v1", true)?.hostname, "api.tprime.vlans.ca");
});

test("rejects credential-bearing and ambiguous AI gateway URLs", () => {
    assert.equal(resolveAiGatewayBaseUrl("https://user:secret@gateway.example.com/v1", false), null);
    assert.equal(resolveAiGatewayBaseUrl("https://gateway.example.com/v1?tenant=a", false), null);
    assert.equal(resolveAiGatewayBaseUrl("javascript:alert(1)", true), null);
});
