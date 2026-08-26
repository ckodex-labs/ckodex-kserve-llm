import type { InferenceInventorySource, LatticeHealthSource } from "@/lib/kserve";
import type { AuditLogSource } from "@/lib/audit.server";
import { getDashboardAttention, isInventoryQualified } from "../dashboard-state";

interface DashboardStatusProps {
    inventory: InferenceInventorySource;
    health: LatticeHealthSource;
    audit: AuditLogSource;
}

function sourceLine(source: string, observedAt: string | null) {
    if (!observedAt) return `${source} · no successful observation`;

    return `${source} · ${new Date(observedAt).toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    })}`;
}

function SummaryRow({ label, value, detail, source }: { label: string; value: string; detail: string; source: string }) {
    return (
        <div className="border-b border-border py-5 last:border-b-0">
            <div className="flex items-baseline justify-between gap-4">
                <dt className="text-sm font-medium">{label}</dt>
                <dd className="font-mono text-sm font-semibold tabular-nums">{value}</dd>
            </div>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">{detail}</p>
            <p className="mt-2 break-words font-mono text-xs text-muted-foreground">{source}</p>
        </div>
    );
}

export function DashboardStatusBlock({ inventory, health, audit }: DashboardStatusProps) {
    const inferences = inventory.data;
    const healthData = health.data;
    const inventoryQualified = isInventoryQualified(inventory);
    const ready = inferences.filter((service) => service.ready).length;
    const progressing = inferences.length - ready;
    const currentGeneration = inferences.filter((service) => (
        service.generation !== null
        && service.observedGeneration !== null
        && service.generation === service.observedGeneration
    )).length;
    const componentsPresent = healthData.components.filter((component) => component.status === "Present").length;
    const attention = getDashboardAttention(inventory, health, audit);

    return (
        <div className="h-full p-6 lg:p-8">
            <div className="flex items-center justify-between gap-4 border-b border-border pb-5">
                <h2 className="text-xl font-semibold">Current state</h2>
                <span className="ck-claim ck-claim--muted">○ source-qualified</span>
            </div>
            <dl>
                <SummaryRow
                    label="Inference readiness"
                    value={inventoryQualified ? `${ready} / ${inferences.length}` : "—"}
                    detail={!inventoryQualified
                        ? inventory.detail
                        : inferences.length === 0
                            ? "The inventory source was observed and returned no services."
                        : progressing === 0
                            ? "Every returned service reports ready."
                            : `${progressing} returned service${progressing === 1 ? " is" : "s are"} still progressing.`}
                    source={sourceLine(inventory.source, inventory.observedAt)}
                />
                <SummaryRow
                    label="Namespace prerequisites"
                    value={health.state === "observed" ? `${componentsPresent} / ${healthData.components.length}` : "—"}
                    detail={health.state === "observed" ? "Presence checks only; component health is not inferred from namespace existence." : health.detail}
                    source={sourceLine(health.source, health.observedAt)}
                />
                <SummaryRow
                    label="Status freshness"
                    value={inventoryQualified && inferences.length > 0 ? `${currentGeneration} / ${inferences.length}` : "—"}
                    detail={inventoryQualified && inferences.length > 0
                        ? "Current means status.observedGeneration matches metadata.generation; missing values remain unqualified."
                        : "Status freshness is not assessable without returned services."}
                    source={sourceLine(inventory.source, inventory.observedAt)}
                />
            </dl>

            <section className="border-t border-border pt-6" id="attention-queue" aria-labelledby="attention-title">
                <div className="flex items-baseline justify-between gap-4">
                    <h3 className="text-base font-semibold" id="attention-title">Requires attention</h3>
                    <span className="font-mono text-sm font-semibold tabular-nums">{attention.length}</span>
                </div>
                {attention.length === 0 ? (
                    <p className="mt-3 text-sm leading-6 text-muted-foreground">
                        No attention items were derived from the qualified sources.
                    </p>
                ) : (
                    <ul className="mt-3 divide-y divide-border border-y border-border">
                        {attention.map((item) => (
                            <li className="py-4" key={item.id}>
                                <p className="text-sm font-semibold">{item.label}</p>
                                <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.detail}</p>
                                <p className="mt-2 break-words font-mono text-xs text-muted-foreground">{item.source}</p>
                            </li>
                        ))}
                    </ul>
                )}
            </section>
        </div>
    );
}
