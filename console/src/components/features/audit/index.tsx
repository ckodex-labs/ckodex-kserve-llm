"use client";

import { useMemo, useState } from "react";

import { useInvestigationSearchState } from "@/hooks/use-investigation-search-state";
import {
    auditEventGlyph,
    auditEventSummary,
    type AuditLogSource,
    type AuditOutcome,
} from "@/lib/audit";
import { OperatorFilterBar } from "@/components/layout/OperatorFilterBar";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

interface LatticeAuditProps {
    audit: AuditLogSource;
    initialOutcome?: OutcomeFilter;
    initialQuery?: string;
}

export type OutcomeFilter = "all" | Lowercase<AuditOutcome>;

function formatTimestamp(value: string) {
    return new Date(value).toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    });
}

function normalizeOutcome(value: OutcomeFilter): AuditOutcome | null {
    if (value === "success") return "Success";
    if (value === "failure") return "Failure";
    if (value === "denied") return "Denied";
    return null;
}

export function LatticeAuditBlock({ audit, initialOutcome = "all", initialQuery = "" }: LatticeAuditProps) {
    const [query, setQuery] = useState(initialQuery);
    const [outcomeFilter, setOutcomeFilter] = useState<OutcomeFilter>(initialOutcome);
    const qualified = audit.state === "observed" || audit.state === "empty";
    const sourceClaim = audit.state === "observed"
        ? "⊢ source observed"
        : audit.state === "empty"
            ? "⊢ empty source observed"
            : audit.state === "malformed"
                ? "○ source partial"
                : "○ source unavailable";
    const deniedOrFailed = audit.data.filter((event) => event.outcome !== "Success").length;
    const actorCount = new Set(audit.data.map((event) => event.actor)).size;
    const hasActiveFilters = query.length > 0 || outcomeFilter !== "all";

    useInvestigationSearchState({ filterKey: "outcome", filterValue: outcomeFilter, query });

    const filtered = useMemo(() => {
        const normalizedQuery = query.trim().toLowerCase();
        const expectedOutcome = normalizeOutcome(outcomeFilter);

        return audit.data.filter((event) => {
            const matchesOutcome = expectedOutcome === null || event.outcome === expectedOutcome;
            if (!matchesOutcome) return false;
            if (!normalizedQuery) return true;

            const searchable = [
                event.action,
                event.resource,
                event.actor,
                event.outcome,
                event.reason,
                event.execId,
                event.execKind,
                event.reproducibilityClass,
                ...Object.entries(event.details).flat(),
            ].join(" ").toLowerCase();
            return searchable.includes(normalizedQuery);
        });
    }, [audit.data, outcomeFilter, query]);

    return (
        <div>
            <div className="flex flex-col justify-between gap-5 border-b border-border p-6 xl:flex-row xl:items-end lg:p-8">
                <div>
                    <p className="ck-label">Operator event ledger</p>
                    <h2 className="mt-2 text-xl font-semibold">Audit records</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Runtime audit events preserve actor, action, outcome, resource, execution identity, and structured context. Records are observed, not cryptographically attested.
                    </p>
                </div>
                <span className={qualified ? "ck-claim" : "ck-claim ck-claim--muted"}>
                    {sourceClaim}
                </span>
            </div>

            <div className="grid gap-px border-b border-border bg-border sm:grid-cols-3">
                <div className="bg-background p-4 lg:px-8">
                    <p className="ck-label">Parsed</p>
                    <p className="mt-2 font-mono text-xl font-semibold tabular-nums">{audit.data.length}</p>
                </div>
                <div className="bg-background p-4">
                    <p className="ck-label">Denied or failed</p>
                    <p className="mt-2 font-mono text-xl font-semibold tabular-nums">{deniedOrFailed}</p>
                </div>
                <div className="bg-background p-4 lg:pr-8">
                    <p className="ck-label">Actors</p>
                    <p className="mt-2 font-mono text-xl font-semibold tabular-nums">{actorCount}</p>
                </div>
            </div>

            <div className="border-b border-border px-6 py-4 lg:px-8">
                <p className="break-words font-mono text-xs text-muted-foreground">{audit.source}</p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{audit.detail}</p>
            </div>

            <OperatorFilterBar
                className="border-x-0 border-t-0"
                fieldsClassName="sm:grid-cols-[minmax(220px,1fr)_180px] lg:px-8"
                hasActiveFilters={hasActiveFilters}
                label="Audit record filters"
                onReset={() => {
                    setQuery("");
                    setOutcomeFilter("all");
                }}
                status={`${filtered.length} of ${audit.data.length} records visible`}
            >
                <label>
                    <span className="ck-label block pb-2">Find record</span>
                    <Input
                        className="h-11"
                        onChange={(event) => setQuery(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === "Escape") setQuery("");
                        }}
                        placeholder="Actor, action, resource, reason, or execution ID"
                        type="search"
                        value={query}
                    />
                </label>
                <label>
                    <span className="ck-label block pb-2">Outcome</span>
                    <NativeSelect
                        className="h-11"
                        onChange={(event) => setOutcomeFilter(event.target.value as OutcomeFilter)}
                        value={outcomeFilter}
                    >
                        <NativeSelectOption value="all">All outcomes</NativeSelectOption>
                        <NativeSelectOption value="success">Success</NativeSelectOption>
                        <NativeSelectOption value="failure">Failure</NativeSelectOption>
                        <NativeSelectOption value="denied">Denied</NativeSelectOption>
                    </NativeSelect>
                </label>
            </OperatorFilterBar>

            <div aria-label="Audit records table" aria-live="polite" className="ck-table-scroll" role="region" tabIndex={0}>
                <table className="w-full min-w-[900px] border-collapse text-left">
                    <thead>
                        <tr className="border-b border-border bg-muted">
                            <th className="ck-label px-6 py-4 lg:px-8" scope="col">Timestamp</th>
                            <th className="ck-label px-6 py-4" scope="col">Outcome</th>
                            <th className="ck-label px-6 py-4" scope="col">Action</th>
                            <th className="ck-label px-6 py-4" scope="col">Resource</th>
                            <th className="ck-label px-6 py-4 lg:pr-8" scope="col">Record</th>
                        </tr>
                    </thead>
                    <tbody>
                        {filtered.length === 0 ? (
                            <tr>
                                <td className="px-6 py-8 text-sm text-muted-foreground lg:px-8" colSpan={5}>
                                    {audit.data.length === 0
                                        ? audit.state === "empty"
                                            ? "The configured audit source was observed and contains no records."
                                            : audit.detail
                                        : "No audit records match the current filters."}
                                </td>
                            </tr>
                        ) : filtered.map((event, index) => (
                            <tr className="border-b border-border align-top last:border-b-0" key={event.execId || `${event.timestamp}-${event.resource}-${index}`}>
                                <td className="px-6 py-4 font-mono text-xs text-muted-foreground lg:px-8">
                                    <time dateTime={event.timestamp}>{formatTimestamp(event.timestamp)}</time>
                                </td>
                                <td className="px-6 py-4 font-mono text-xs">
                                    <span className="mr-2" aria-hidden="true">{auditEventGlyph(event.outcome)}</span>
                                    {event.outcome}
                                </td>
                                <td className="px-6 py-4 font-mono text-xs font-semibold">{event.action}</td>
                                <td className="max-w-[260px] break-words px-6 py-4 font-mono text-xs">{event.resource}</td>
                                <td className="px-6 py-4 text-sm leading-6 lg:pr-8">
                                    <p>{auditEventSummary(event)}</p>
                                    <p className="mt-1 font-mono text-[11px] text-muted-foreground">Actor: {event.actor}</p>
                                    {(event.execId || Object.keys(event.details).length > 0) ? (
                                        <details className="mt-3 border border-border p-3">
                                            <summary className="min-h-11 cursor-pointer py-2 font-mono text-xs font-semibold">Inspect structured record</summary>
                                            <dl className="mt-2 grid gap-3 border-t border-border pt-3">
                                                {event.execId ? (
                                                    <div>
                                                        <dt className="ck-label">Execution ID</dt>
                                                        <dd className="mt-1 break-all font-mono text-xs">{event.execId}</dd>
                                                    </div>
                                                ) : null}
                                                {event.execKind ? (
                                                    <div>
                                                        <dt className="ck-label">Execution kind</dt>
                                                        <dd className="mt-1 font-mono text-xs">{event.execKind}</dd>
                                                    </div>
                                                ) : null}
                                                {event.reproducibilityClass ? (
                                                    <div>
                                                        <dt className="ck-label">Reproducibility</dt>
                                                        <dd className="mt-1 font-mono text-xs">{event.reproducibilityClass}</dd>
                                                    </div>
                                                ) : null}
                                                {Object.entries(event.details).map(([key, value]) => (
                                                    <div key={key}>
                                                        <dt className="ck-label">{key.replaceAll("_", " ")}</dt>
                                                        <dd className="mt-1 break-words font-mono text-xs">{value}</dd>
                                                    </div>
                                                ))}
                                            </dl>
                                        </details>
                                    ) : null}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
