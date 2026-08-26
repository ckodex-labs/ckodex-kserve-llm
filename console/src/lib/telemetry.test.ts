import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
    formatTelemetryValue,
    parsePrometheusAlerts,
    parsePrometheusMatrix,
    resolvePrometheusBaseUrl,
    safeTelemetryLink,
} from "./telemetry.ts";

function fixture(name: string) {
    return JSON.parse(readFileSync(new URL(`../../test/fixtures/${name}`, import.meta.url), "utf8")) as unknown;
}

test("parses only approved CKODEX and vLLM series with projected display labels", () => {
    const parsed = parsePrometheusMatrix(fixture("prometheus-range.json"));

    assert.ok(parsed);
    assert.equal(parsed.series.length, 3);
    assert.equal(parsed.series[0].latest, 0.99);
    assert.deepEqual(parsed.series[0].labels, { model: "model-a" });
    assert.equal(parsed.series[2].metric, "vllm:kv_cache_usage_perc");
    assert.deepEqual(parsed.series[2].labels, {
        model_name: "gemma-4-26b-nvfp4",
        namespace: "cortaix-llm-inference",
        service: "gemma4",
    });
    assert.deepEqual(parsed.warnings, ["one scrape was incomplete"]);
});

test("parses active Prometheus alerts without treating severity as an emergency state", () => {
    const parsed = parsePrometheusAlerts(fixture("prometheus-alerts.json"));

    assert.ok(parsed);
    assert.equal(parsed.alerts.length, 1);
    assert.equal(parsed.alerts[0].name, "LLMReconcileErrorRate");
    assert.equal(parsed.alerts[0].state, "firing");
    assert.equal(parsed.alerts[0].labels.severity, "warning");
});

test("rejects malformed Prometheus envelopes", () => {
    assert.equal(parsePrometheusMatrix({ status: "error", data: {} }), null);
    assert.equal(parsePrometheusAlerts({ status: "success", data: { alerts: "invalid" } }), null);
});

test("formats metric values using their declared units", () => {
    assert.equal(formatTelemetryValue("ckodex:reconcile_success_rate", 0.991), "99.1%");
    assert.equal(formatTelemetryValue("ckodex:inference_latency_p99", 1.35), "1.35 s");
    assert.equal(formatTelemetryValue("ckodex:tokens_per_tenant_hour", 1200), "1,200");
    assert.equal(formatTelemetryValue("vllm:kv_cache_usage_perc", 0.512), "51.2%");
});

test("requires HTTPS unless insecure transport is explicitly allowed", () => {
    assert.equal(resolvePrometheusBaseUrl("https://prometheus.example.com/base/", false)?.toString(), "https://prometheus.example.com/base");
    assert.equal(resolvePrometheusBaseUrl("http://127.0.0.1:9090", false)?.toString(), "http://127.0.0.1:9090/");
    assert.equal(resolvePrometheusBaseUrl("http://prometheus.monitoring.svc:9090", false), null);
    assert.equal(resolvePrometheusBaseUrl("http://prometheus.monitoring.svc:9090", true)?.hostname, "prometheus.monitoring.svc");
    assert.equal(resolvePrometheusBaseUrl("https://user:secret@prometheus.example.com", false), null);
});

test("allows only HTTP and HTTPS runbook links", () => {
    assert.equal(safeTelemetryLink("https://docs.ckodex.com/runbook"), "https://docs.ckodex.com/runbook");
    assert.equal(safeTelemetryLink("javascript:alert(1)"), "");
});
