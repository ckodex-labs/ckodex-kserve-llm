import type { InferenceInventorySource, InferenceService } from "@/lib/kserve";

interface DashboardInventoryProps {
    inventory: InferenceInventorySource;
}

function StateField({ label, value }: { label: string; value: string }) {
    return (
        <div>
            <dt className="ck-label">{label}</dt>
            <dd className="mt-2 break-words font-mono text-xs font-semibold">{value || "Not reported"}</dd>
        </div>
    );
}

function InferenceRow({ inference }: { inference: InferenceService }) {
    return (
        <article className="border-b border-border p-6 last:border-b-0 lg:p-8">
            <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
                <div className="min-w-0">
                    <h3 className="break-words text-lg font-semibold">{inference.name}</h3>
                    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">ns/{inference.namespace}</p>
                    <p className="mt-2 break-all font-mono text-xs leading-5 text-muted-foreground">{inference.uri}</p>
                </div>
                <span className={inference.ready ? "ck-claim" : "ck-claim ck-claim--muted"}>
                    {inference.ready ? "⊢ ready reported" : "⊢ not ready reported"}
                </span>
            </div>

            <dl className="mt-6 grid grid-cols-2 gap-5 border-t border-border pt-5 md:grid-cols-5">
                <StateField label="Ready / desired" value={`${inference.replicas} / ${inference.desiredReplicas}`} />
                <StateField label="Autoscale ceiling" value={String(inference.maxReplicas)} />
                <StateField label="Generation" value={`${inference.observedGeneration ?? "—"} / ${inference.generation ?? "—"}`} />
                <StateField label="Conditions" value={String(inference.conditions.length)} />
                <div>
                    <dt className="ck-label">Endpoint</dt>
                    <dd className="mt-2 text-xs">
                        {inference.url ? (
                            <a className="ck-text-action font-mono" href={inference.url} rel="noreferrer" target="_blank">Open endpoint</a>
                        ) : (
                            <span className="font-mono text-muted-foreground">not returned</span>
                        )}
                    </dd>
                </div>
            </dl>
        </article>
    );
}

export function DashboardInventoryBlock({ inventory }: DashboardInventoryProps) {
    const inferences = inventory.data;
    const qualified = inventory.state === "observed" || inventory.state === "empty";
    const sourceClaim = inventory.state === "observed"
        ? `⊢ ${inferences.length} returned`
        : inventory.state === "empty"
            ? "⊢ empty observed"
            : inventory.state === "malformed"
                ? "○ source partial"
                : "○ source unavailable";

    return (
        <div className="h-full">
            <div className="border-b border-border p-6 lg:p-8">
                <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
                    <div>
                        <h2 className="text-xl font-semibold">LLMInferenceService inventory</h2>
                        <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                            Desired model-serving resources and the latest status fields returned by the Kubernetes API.
                        </p>
                    </div>
                    <span className={qualified ? "ck-claim" : "ck-claim ck-claim--muted"}>{sourceClaim}</span>
                </div>
                <p className="mt-4 break-words font-mono text-xs text-muted-foreground">
                    {inventory.source}
                </p>
            </div>

            {!qualified ? (
                <div className="m-6 border border-dashed border-border p-8 lg:m-8">
                    <h3 className="text-base font-semibold">Inventory source unavailable</h3>
                    <p className="mt-3 max-w-[65ch] text-sm leading-6 text-muted-foreground">
                        {inventory.detail}
                    </p>
                </div>
            ) : inferences.length === 0 ? (
                <div className="m-6 border border-dashed border-border p-8 lg:m-8">
                    <h3 className="text-base font-semibold">No services declared</h3>
                    <p className="mt-3 max-w-[65ch] text-sm leading-6 text-muted-foreground">
                        {inventory.detail}
                    </p>
                </div>
            ) : (
                <div>{inferences.map((inference) => <InferenceRow key={inference.name} inference={inference} />)}</div>
            )}
        </div>
    );
}
