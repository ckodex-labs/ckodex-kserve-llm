import type { SourceSnapshot } from "./source";

export const telemetryMetricNames = [
    "ckodex:reconcile_success_rate",
    "ckodex:inference_latency_p99",
    "ckodex:gpu_utilization_p95",
    "ckodex:tokens_per_tenant_hour",
    "vllm:num_requests_running",
    "vllm:num_requests_waiting",
    "vllm:kv_cache_usage_perc",
] as const;

const telemetryDisplayLabelNames = new Set([
    "model",
    "model_name",
    "namespace",
    "node",
    "service",
    "tenant",
]);

export type TelemetryMetricName = typeof telemetryMetricNames[number];

export interface TelemetryPoint {
    at: string;
    value: number;
}

export interface TelemetrySeries {
    id: string;
    metric: TelemetryMetricName;
    labels: Record<string, string>;
    points: TelemetryPoint[];
    latest: number;
}

export interface TelemetryAlert {
    id: string;
    name: string;
    state: string;
    value: number | null;
    activeAt: string | null;
    labels: Record<string, string>;
    annotations: Record<string, string>;
}

export interface TelemetryWindow {
    start: string;
    end: string;
    stepSeconds: number;
}

export interface TelemetryData {
    series: TelemetrySeries[];
    alerts: TelemetryAlert[];
    warnings: string[];
    window: TelemetryWindow;
}

export type TelemetrySource = SourceSnapshot<TelemetryData>;

interface ParsedMatrix {
    series: TelemetrySeries[];
    warnings: string[];
}

interface ParsedAlerts {
    alerts: TelemetryAlert[];
    warnings: string[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringMap(value: unknown): Record<string, string> | null {
    if (!isRecord(value)) return null;
    const result: Record<string, string> = {};
    for (const [key, item] of Object.entries(value)) {
        if (typeof item !== "string") return null;
        result[key] = item;
    }
    return result;
}

function warningsFrom(value: Record<string, unknown>) {
    if (value.warnings === undefined) return [];
    if (!Array.isArray(value.warnings) || !value.warnings.every((warning) => typeof warning === "string")) return null;
    return value.warnings;
}

function isoFromUnixSeconds(value: unknown) {
    if (typeof value !== "number" || !Number.isFinite(value)) return null;
    const date = new Date(value * 1000);
    return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

function finiteNumber(value: unknown) {
    if (typeof value !== "string" && typeof value !== "number") return null;
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
}

function stableLabels(labels: Record<string, string>) {
    return Object.entries(labels)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, value]) => `${key}=${value}`)
        .join(",");
}

function isTelemetryMetric(value: string): value is TelemetryMetricName {
    return telemetryMetricNames.includes(value as TelemetryMetricName);
}

export function parsePrometheusMatrix(value: unknown): ParsedMatrix | null {
    if (!isRecord(value) || value.status !== "success" || !isRecord(value.data)) return null;
    if (value.data.resultType !== "matrix" || !Array.isArray(value.data.result)) return null;
    const warnings = warningsFrom(value);
    if (warnings === null) return null;

    const series: TelemetrySeries[] = [];
    for (const item of value.data.result) {
        if (!isRecord(item) || !Array.isArray(item.values)) return null;
        const labels = stringMap(item.metric);
        if (labels === null) return null;
        const metric = labels.__name__;
        if (!metric || !isTelemetryMetric(metric)) continue;

        const points: TelemetryPoint[] = [];
        for (const point of item.values) {
            if (!Array.isArray(point) || point.length !== 2) return null;
            const at = isoFromUnixSeconds(point[0]);
            const sample = finiteNumber(point[1]);
            if (at !== null && sample !== null) points.push({ at, value: sample });
        }
        if (points.length === 0) continue;

        const displayLabels = Object.fromEntries(Object.entries(labels).filter(([key]) => telemetryDisplayLabelNames.has(key)));
        series.push({
            id: `${metric}{${stableLabels(displayLabels)}}`,
            metric,
            labels: displayLabels,
            points,
            latest: points.at(-1)!.value,
        });
    }

    return { series, warnings };
}

export function parsePrometheusAlerts(value: unknown): ParsedAlerts | null {
    if (!isRecord(value) || value.status !== "success" || !isRecord(value.data) || !Array.isArray(value.data.alerts)) return null;
    const warnings = warningsFrom(value);
    if (warnings === null) return null;

    const alerts: TelemetryAlert[] = [];
    for (const item of value.data.alerts) {
        if (!isRecord(item)) return null;
        const labels = stringMap(item.labels);
        const annotations = stringMap(item.annotations);
        if (labels === null || annotations === null || typeof item.state !== "string") return null;

        const name = labels.alertname;
        if (!name) return null;
        const activeAt = typeof item.activeAt === "string" && !Number.isNaN(Date.parse(item.activeAt))
            ? new Date(item.activeAt).toISOString()
            : null;
        const alertLabels = Object.fromEntries(Object.entries(labels).filter(([key]) => key !== "alertname"));
        alerts.push({
            id: `${name}{${stableLabels(alertLabels)}}@${activeAt || "unknown"}`,
            name,
            state: item.state,
            value: finiteNumber(item.value),
            activeAt,
            labels: alertLabels,
            annotations,
        });
    }

    return { alerts, warnings };
}

export function formatTelemetryValue(metric: TelemetryMetricName, value: number) {
    if (metric === "ckodex:reconcile_success_rate" || metric === "ckodex:gpu_utilization_p95" || metric === "vllm:kv_cache_usage_perc") {
        return new Intl.NumberFormat(undefined, { style: "percent", maximumFractionDigits: 1 }).format(value);
    }
    if (metric === "ckodex:inference_latency_p99") {
        return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(value)} s`;
    }
    return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
}

export function telemetryMetricLabel(metric: TelemetryMetricName) {
    if (metric === "ckodex:reconcile_success_rate") return "Reconcile success rate";
    if (metric === "ckodex:inference_latency_p99") return "Inference latency p99";
    if (metric === "ckodex:gpu_utilization_p95") return "GPU utilization p95";
    if (metric === "vllm:num_requests_running") return "Requests running";
    if (metric === "vllm:num_requests_waiting") return "Requests waiting";
    if (metric === "vllm:kv_cache_usage_perc") return "KV cache utilization";
    return "Tenant tokens / hour";
}

export function resolvePrometheusBaseUrl(value: string | undefined, allowInsecure: boolean) {
    if (!value) return null;
    try {
        const url = new URL(value);
        const loopback = url.hostname === "127.0.0.1" || url.hostname === "localhost" || url.hostname === "[::1]";
        if (url.username || url.password || url.search || url.hash) return null;
        if (url.protocol !== "https:" && !(url.protocol === "http:" && (loopback || allowInsecure))) return null;
        if (url.protocol !== "http:" && url.protocol !== "https:") return null;
        url.pathname = url.pathname.replace(/\/+$/, "");
        return url;
    } catch {
        return null;
    }
}

export function safeTelemetryLink(value: string | undefined) {
    if (!value) return "";
    try {
        const url = new URL(value);
        return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : "";
    } catch {
        return "";
    }
}
