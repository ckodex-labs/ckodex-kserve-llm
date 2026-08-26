"use client";

import { useMemo, useState } from "react";

import { useInvestigationSearchState } from "@/hooks/use-investigation-search-state";
import {
    formatTelemetryValue,
    safeTelemetryLink,
    telemetryMetricLabel,
    type TelemetryPoint,
    type TelemetrySource,
} from "@/lib/telemetry";
import { OperatorFilterBar } from "@/components/layout/OperatorFilterBar";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

interface TelemetryViewProps {
    initialState?: AlertStateFilter;
    source: TelemetrySource;
    initialQuery?: string;
}

export type AlertStateFilter = "all" | "firing" | "pending";

function absoluteTime(value: string | null) {
    if (!value) return "Not reported";
    return new Date(value).toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    });
}

function labelText(labels: Record<string, string>) {
    const entries = Object.entries(labels).sort(([left], [right]) => left.localeCompare(right));
    return entries.length > 0 ? entries.map(([key, value]) => `${key}=${value}`).join(" · ") : "Aggregate series";
}

function Sparkline({ label, points }: { label: string; points: TelemetryPoint[] }) {
    if (points.length < 2) {
        return <p className="font-mono text-[11px] text-muted-foreground">Single sample; trend withheld.</p>;
    }

    const width = 240;
    const height = 56;
    const values = points.map((point) => point.value);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = max - min || 1;
    const polyline = points.map((point, index) => {
        const x = (index / (points.length - 1)) * width;
        const y = height - ((point.value - min) / range) * (height - 8) - 4;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(" ");

    return (
        <svg aria-label={`${label}; ${points.length} observed samples`} className="h-14 w-full text-foreground" preserveAspectRatio="none" role="img" viewBox={`0 0 ${width} ${height}`}>
            <line className="text-border" stroke="currentColor" strokeWidth="1" vectorEffect="non-scaling-stroke" x1="0" x2={width} y1={height - 1} y2={height - 1} />
            <polyline fill="none" points={polyline} stroke="currentColor" strokeLinejoin="miter" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
        </svg>
    );
}

export function TelemetryView({ source, initialQuery = "", initialState = "all" }: TelemetryViewProps) {
    const [query, setQuery] = useState(initialQuery);
    const [stateFilter, setStateFilter] = useState<AlertStateFilter>(initialState);
    const hasActiveFilters = query.length > 0 || stateFilter !== "all";
    useInvestigationSearchState({ filterKey: "state", filterValue: stateFilter, query });
    const filteredSeries = useMemo(() => {
        const normalizedQuery = query.trim().toLowerCase();
        if (!normalizedQuery) return source.data.series;
        return source.data.series.filter((series) => [
            series.metric,
            telemetryMetricLabel(series.metric),
            ...Object.entries(series.labels).flat(),
        ].join(" ").toLowerCase().includes(normalizedQuery));
    }, [query, source.data.series]);
    const filteredAlerts = useMemo(() => {
        const normalizedQuery = query.trim().toLowerCase();
        return source.data.alerts.filter((alert) => {
            if (stateFilter !== "all" && alert.state.toLowerCase() !== stateFilter) return false;
            if (!normalizedQuery) return true;
            const text = [
                alert.name,
                alert.state,
                ...Object.entries(alert.labels).flat(),
                ...Object.entries(alert.annotations).flat(),
            ].join(" ").toLowerCase();
            return text.includes(normalizedQuery);
        });
    }, [query, source.data.alerts, stateFilter]);

    return (
        <div>
            <OperatorFilterBar
                className="mb-8"
                fieldsClassName="ck-telemetry-filter-fields"
                hasActiveFilters={hasActiveFilters}
                label="Telemetry investigation filters"
                onReset={() => {
                    setQuery("");
                    setStateFilter("all");
                }}
                status={`${filteredSeries.length} of ${source.data.series.length} series · ${filteredAlerts.length} of ${source.data.alerts.length} alerts visible`}
            >
                <label>
                    <span className="ck-label block pb-2">Find signal</span>
                    <Input
                        className="h-11"
                        onChange={(event) => setQuery(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === "Escape") setQuery("");
                        }}
                        placeholder="Metric, alert, model, team, or annotation"
                        type="search"
                        value={query}
                    />
                </label>
                <label>
                    <span className="ck-label block pb-2">Alert state</span>
                    <NativeSelect
                        className="h-11"
                        onChange={(event) => setStateFilter(event.target.value as AlertStateFilter)}
                        value={stateFilter}
                    >
                        <NativeSelectOption value="all">All active states</NativeSelectOption>
                        <NativeSelectOption value="firing">Firing</NativeSelectOption>
                        <NativeSelectOption value="pending">Pending</NativeSelectOption>
                    </NativeSelect>
                </label>
            </OperatorFilterBar>

            <section aria-labelledby="metric-series-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Allowlisted operator signals</p>
                    <h2 className="mt-2 text-xl font-semibold" id="metric-series-title">Metric series</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Fixed CKODEX recording rules and bounded vLLM gauges over the last 60 minutes at 60-second resolution. Missing series remain absent rather than inferred as zero.
                    </p>
                </div>
                <div className="mt-px border border-border">
                    {filteredSeries.length === 0 ? (
                        <p className="p-6 text-sm leading-6 text-muted-foreground">
                            {source.state === "unavailable"
                                ? source.detail
                                : source.data.series.length === 0
                                    ? "The Prometheus query returned no allowlisted metric series."
                                    : "No metric series match the current investigation filter."}
                        </p>
                    ) : filteredSeries.map((series) => (
                        <article className="grid gap-5 border-b border-border p-5 last:border-b-0 xl:grid-cols-[minmax(220px,0.9fr)_minmax(260px,1.2fr)_150px] xl:items-center" key={series.id}>
                            <div>
                                <h3 className="text-sm font-semibold">{telemetryMetricLabel(series.metric)}</h3>
                                <p className="mt-2 break-words font-mono text-[11px] text-muted-foreground">{labelText(series.labels)}</p>
                                <p className="mt-1 font-mono text-[11px] text-muted-foreground">{series.points.length} samples · last {absoluteTime(series.points.at(-1)?.at || null)}</p>
                            </div>
                            <Sparkline label={telemetryMetricLabel(series.metric)} points={series.points} />
                            <div className="xl:text-right">
                                <p className="ck-label">Latest observed</p>
                                <p className="mt-2 font-mono text-2xl font-semibold tabular-nums">{formatTelemetryValue(series.metric, series.latest)}</p>
                            </div>
                        </article>
                    ))}
                </div>
            </section>

            <section className="mt-8" aria-labelledby="active-alerts-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Prometheus active-alert API</p>
                    <h2 className="mt-2 text-xl font-semibold" id="active-alerts-title">Alert triage</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Pending and firing alert observations. Alert severity does not assert an active CKODEX emergency protocol.
                    </p>
                </div>
                <div className="mt-px border border-border" aria-live="polite">
                    {filteredAlerts.length === 0 ? (
                        <p className="p-6 text-sm leading-6 text-muted-foreground">
                            {source.data.alerts.length === 0 ? "No active alerts were returned." : "No active alerts match the current filters."}
                        </p>
                    ) : filteredAlerts.map((alert) => {
                        const runbookUrl = safeTelemetryLink(alert.annotations.runbook_url);
                        return (
                            <article className="grid gap-5 border-b border-border p-5 last:border-b-0 xl:grid-cols-[minmax(180px,0.7fr)_minmax(280px,1.4fr)_180px]" key={alert.id}>
                                <div>
                                    <p className="font-mono text-xs font-semibold">⊢ {alert.state} observed</p>
                                    <h3 className="mt-2 text-sm font-semibold">{alert.name}</h3>
                                    <p className="mt-1 font-mono text-[11px] text-muted-foreground">Active at {absoluteTime(alert.activeAt)}</p>
                                </div>
                                <div>
                                    <p className="text-sm leading-6">{alert.annotations.summary || alert.annotations.description || "No summary returned."}</p>
                                    <p className="mt-2 break-words font-mono text-[11px] text-muted-foreground">{labelText(alert.labels)}</p>
                                </div>
                                <div>
                                    {alert.value !== null ? <p className="font-mono text-xs tabular-nums">Value {alert.value}</p> : null}
                                    {runbookUrl ? <a className="ck-text-action mt-3" href={runbookUrl} rel="noreferrer" target="_blank">Open runbook</a> : null}
                                </div>
                            </article>
                        );
                    })}
                </div>
            </section>

            {source.data.warnings.length > 0 ? (
                <section className="mt-8 border border-dashed border-border p-5" aria-labelledby="prometheus-warnings-title">
                    <p className="ck-label">Source annotations</p>
                    <h2 className="mt-2 text-base font-semibold" id="prometheus-warnings-title">Prometheus warnings</h2>
                    <ul className="mt-3 divide-y divide-border border-y border-border">
                        {source.data.warnings.map((warning, index) => <li className="py-3 text-sm leading-6" key={`${warning}-${index}`}>○ {warning}</li>)}
                    </ul>
                </section>
            ) : null}
        </div>
    );
}
