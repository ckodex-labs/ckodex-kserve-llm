"use client";

import { useMemo, useState } from "react";

import { useInvestigationSearchState } from "@/hooks/use-investigation-search-state";
import { Link } from "@/i18n/routing";
import type { InferenceInventorySource } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";
import { OperatorFilterBar } from "@/components/layout/OperatorFilterBar";
import { SourceBoundaryPanel } from "@/components/layout/SourceBoundaryPanel";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

interface FleetViewProps {
    initialQuery?: string;
    initialReadiness?: ReadinessFilter;
    inventory: InferenceInventorySource;
}

type ReadinessFilter = "all" | "ready" | "attention";

export function FleetViewBlock({ initialQuery = "", initialReadiness = "all", inventory }: FleetViewProps) {
    const [query, setQuery] = useState(initialQuery);
    const [readiness, setReadiness] = useState<ReadinessFilter>(initialReadiness);
    const sourceQualified = isSourceObserved(inventory);
    const hasActiveFilters = query.length > 0 || readiness !== "all";

    useInvestigationSearchState({ filterKey: "readiness", filterValue: readiness, query });

    const filtered = useMemo(() => {
        const normalizedQuery = query.trim().toLowerCase();
        return inventory.data.filter((service) => {
            const matchesQuery = normalizedQuery.length === 0
                || service.name.toLowerCase().includes(normalizedQuery)
                || service.uri.toLowerCase().includes(normalizedQuery)
                || service.namespace.toLowerCase().includes(normalizedQuery)
                || service.modelRevision.toLowerCase().includes(normalizedQuery)
                || service.conditions.some((condition) => condition.type.toLowerCase().includes(normalizedQuery));
            const matchesReadiness = readiness === "all"
                || (readiness === "ready" && service.ready)
                || (readiness === "attention" && !service.ready);
            return matchesQuery && matchesReadiness;
        });
    }, [inventory.data, query, readiness]);

    if (!sourceQualified) {
        return <SourceBoundaryPanel label="fleet" snapshot={inventory} title="Workload inventory unavailable" />;
    }

    return (
        <section aria-labelledby="fleet-manifest-title">
            <OperatorFilterBar
                fieldsClassName="ck-fleet-filter-fields"
                hasActiveFilters={hasActiveFilters}
                label="Inference fleet filters"
                onReset={() => {
                    setQuery("");
                    setReadiness("all");
                }}
                status={`${filtered.length} of ${inventory.data.length} services visible`}
            >
                <div>
                    <p className="ck-label">Observed manifest</p>
                    <h2 className="mt-2 text-xl font-semibold" id="fleet-manifest-title">Inference services</h2>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Filter the current source snapshot. Operational mutations remain manifest-driven and are not exposed here.
                    </p>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                    <label>
                        <span className="ck-label block pb-2">Find workload</span>
                        <Input
                            className="h-11 min-w-[220px]"
                            onChange={(event) => setQuery(event.target.value)}
                            onKeyDown={(event) => {
                                if (event.key === "Escape") setQuery("");
                            }}
                            placeholder="Name, URI, or trust state"
                            type="search"
                            value={query}
                        />
                    </label>
                    <label>
                        <span className="ck-label block pb-2">Readiness</span>
                        <NativeSelect
                            className="h-11"
                            onChange={(event) => setReadiness(event.target.value as ReadinessFilter)}
                            value={readiness}
                        >
                            <NativeSelectOption value="all">All services</NativeSelectOption>
                            <NativeSelectOption value="ready">Ready reported</NativeSelectOption>
                            <NativeSelectOption value="attention">Needs attention</NativeSelectOption>
                        </NativeSelect>
                    </label>
                </div>
            </OperatorFilterBar>

            <div className="mt-px border border-border" aria-live="polite">
                <div className="hidden grid-cols-[minmax(180px,1.2fr)_minmax(180px,1.5fr)_110px_130px_116px] gap-4 border-b border-border bg-muted px-5 py-3 lg:grid">
                    <span className="ck-label">Workload</span>
                    <span className="ck-label">Model source</span>
                    <span className="ck-label">Readiness</span>
                    <span className="ck-label">Replicas</span>
                    <span className="ck-label">Record</span>
                </div>

                {filtered.length === 0 ? (
                    <div className="p-10 text-center">
                        <p className="text-base font-semibold">No matching services</p>
                        <p className="mt-2 text-sm text-muted-foreground">
                            {inventory.data.length === 0 ? "The cluster-scoped observation contains no LLMInferenceService resources." : "Change the search or readiness filter."}
                        </p>
                    </div>
                ) : (
                    filtered.map((service) => (
                        <article className="border-b border-border p-5 last:border-b-0" key={`${service.namespace}/${service.name}`}>
                            <div className="grid gap-5 lg:grid-cols-[minmax(180px,1.2fr)_minmax(180px,1.5fr)_110px_130px_116px] lg:items-center">
                                <div className="min-w-0">
                                    <p className="ck-label lg:hidden">Workload</p>
                                    <p className="truncate font-mono text-sm font-semibold">{service.name}</p>
                                    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">ns/{service.namespace}</p>
                                </div>
                                <div className="min-w-0">
                                    <p className="ck-label lg:hidden">Model source</p>
                                    <p className="truncate font-mono text-xs text-muted-foreground" title={service.uri}>{service.uri}</p>
                                </div>
                                <div>
                                    <p className="ck-label lg:hidden">Readiness</p>
                                    <span className={service.ready ? "ck-claim" : "ck-claim ck-claim--muted"}>
                                        {service.ready ? "⊢ ready" : "○ not ready"}
                                    </span>
                                </div>
                                <div>
                                    <p className="ck-label lg:hidden">Replicas</p>
                                    <p className="font-mono text-sm tabular-nums">{service.replicas} / {service.desiredReplicas}</p>
                                </div>
                                <Link
                                    className="flex min-h-11 items-center justify-center border border-border px-3 text-sm font-semibold hover:bg-muted"
                                    href={`/fleet/${encodeURIComponent(service.namespace)}/${encodeURIComponent(service.name)}` as never}
                                >
                                    Open record
                                </Link>
                            </div>
                        </article>
                    ))
                )}
            </div>
        </section>
    );
}
