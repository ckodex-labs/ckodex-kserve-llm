import "server-only";

import {
    parsePrometheusAlerts,
    parsePrometheusMatrix,
    resolvePrometheusBaseUrl,
    telemetryMetricNames,
    type TelemetryData,
    type TelemetrySource,
} from "./telemetry";

const queryWindowSeconds = 60 * 60;
const queryStepSeconds = 60;
const requestTimeoutMs = 5_000;
const maxResponseCharacters = 2_000_000;

interface PrometheusConfig {
    baseUrl: URL;
    bearerToken: string;
}

function getPrometheusConfig(): PrometheusConfig | null {
    const allowInsecure = process.env.CKODEX_PROMETHEUS_ALLOW_INSECURE === "true";
    const baseUrl = resolvePrometheusBaseUrl(process.env.CKODEX_PROMETHEUS_URL, allowInsecure);
    if (!baseUrl) return null;
    return {
        baseUrl,
        bearerToken: process.env.CKODEX_PROMETHEUS_BEARER_TOKEN || "",
    };
}

function endpoint(config: PrometheusConfig, path: string, parameters?: Record<string, string>) {
    const basePath = config.baseUrl.pathname.replace(/\/$/, "");
    const url = new URL(`${basePath}${path}`, config.baseUrl.origin);
    for (const [key, value] of Object.entries(parameters || {})) url.searchParams.set(key, value);
    return url;
}

async function getJson(config: PrometheusConfig, path: string, parameters?: Record<string, string>) {
    const headers: HeadersInit = { Accept: "application/json" };
    if (config.bearerToken) headers.Authorization = `Bearer ${config.bearerToken}`;

    const response = await fetch(endpoint(config, path, parameters), {
        cache: "no-store",
        headers,
        redirect: "error",
        signal: AbortSignal.timeout(requestTimeoutMs),
    });
    const declaredLength = Number(response.headers.get("content-length") || "0");
    if (Number.isFinite(declaredLength) && declaredLength > maxResponseCharacters) {
        throw new Error("Prometheus response exceeded the configured bound.");
    }
    if (!response.ok) throw new Error(`Prometheus returned HTTP ${response.status}.`);

    const body = await response.text();
    if (body.length > maxResponseCharacters) throw new Error("Prometheus response exceeded the configured bound.");
    return JSON.parse(body) as unknown;
}

export async function getTelemetrySource(now: Date = new Date()): Promise<TelemetrySource> {
    const config = getPrometheusConfig();
    const endSeconds = Math.floor(now.getTime() / 1000);
    const startSeconds = endSeconds - queryWindowSeconds;
    const window = {
        start: new Date(startSeconds * 1000).toISOString(),
        end: new Date(endSeconds * 1000).toISOString(),
        stepSeconds: queryStepSeconds,
    };
    const emptyData: TelemetryData = { series: [], alerts: [], warnings: [], window };

    if (!config) {
        return {
            state: "unavailable",
            data: emptyData,
            source: "Prometheus HTTP API v1",
            observedAt: null,
            detail: "No valid Prometheus URL is configured. HTTPS is required unless insecure transport is explicitly enabled for a trusted internal endpoint.",
        };
    }

    const metricSelector = `{__name__=~"${telemetryMetricNames.join("|")}"}`;
    const results = await Promise.allSettled([
        getJson(config, "/api/v1/query_range", {
            query: metricSelector,
            start: String(startSeconds),
            end: String(endSeconds),
            step: `${queryStepSeconds}s`,
            timeout: "5s",
            limit: "500",
        }),
        getJson(config, "/api/v1/alerts"),
    ]);

    const matrix = results[0].status === "fulfilled" ? parsePrometheusMatrix(results[0].value) : null;
    const alertSet = results[1].status === "fulfilled" ? parsePrometheusAlerts(results[1].value) : null;
    const failedRequests = Number(matrix === null) + Number(alertSet === null);
    const warnings = [...(matrix?.warnings || []), ...(alertSet?.warnings || [])];
    const data: TelemetryData = {
        series: matrix?.series || [],
        alerts: alertSet?.alerts || [],
        warnings,
        window,
    };
    const returned = data.series.length + data.alerts.length;
    const source = `${config.baseUrl.origin}${config.baseUrl.pathname || ""} · Prometheus HTTP API v1`;

    return {
        state: failedRequests === results.length
            ? "unavailable"
            : failedRequests > 0 || warnings.length > 0
                ? "partial"
                : returned === 0 ? "empty" : "observed",
        data,
        source,
        observedAt: failedRequests < results.length ? now.toISOString() : null,
        detail: failedRequests > 0
            ? `${results.length - failedRequests} of ${results.length} telemetry requests succeeded. Returned ${data.series.length} metric series and ${data.alerts.length} active alerts.`
            : `Returned ${data.series.length} allowlisted metric series and ${data.alerts.length} active alerts over a 60-minute window.${warnings.length > 0 ? ` Prometheus returned ${warnings.length} warning annotation${warnings.length === 1 ? "" : "s"}.` : ""}`,
    };
}
