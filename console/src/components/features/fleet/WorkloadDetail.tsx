import { Link } from "@/i18n/routing";
import type { InferenceService } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";
import type { WorkloadRuntimeSource } from "@/lib/workload.server";

interface WorkloadDetailProps {
    service: InferenceService | null;
    runtime: WorkloadRuntimeSource;
}

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

export function WorkloadDetail({ service, runtime }: WorkloadDetailProps) {
    const runtimeQualified = isSourceObserved(runtime);

    return (
        <div>
            <Link className="ck-text-action" href="/fleet">← Back to inference fleet</Link>

            {service ? (
                <nav className="mt-6 grid gap-px border border-border bg-border sm:grid-cols-2" aria-label="Workload investigation paths">
                    <Link
                        className="min-h-16 bg-background p-4 hover:bg-muted"
                        href={`/events?q=${encodeURIComponent(`${service.namespace}/${service.name}`)}` as never}
                    >
                        <span className="ck-label block">Audit correlation</span>
                        <span className="mt-2 block text-sm font-semibold">Inspect workload records →</span>
                    </Link>
                    <Link
                        className="min-h-16 bg-background p-4 hover:bg-muted"
                        href={`/telemetry?q=${encodeURIComponent(service.name)}` as never}
                    >
                        <span className="ck-label block">Measurement correlation</span>
                        <span className="mt-2 block text-sm font-semibold">Inspect workload signals →</span>
                    </Link>
                </nav>
            ) : null}

            {!service ? (
                <section className="ck-quiet mt-6 p-6" aria-labelledby="workload-unavailable-title">
                    <p className="ck-label">Inventory source</p>
                    <h2 className="mt-3 text-2xl font-semibold" id="workload-unavailable-title">Workload record unavailable</h2>
                    <p className="mt-3 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        The inventory source did not return a qualified workload record. Runtime context below remains independently qualified.
                    </p>
                </section>
            ) : (
                <section className="mt-6" aria-labelledby="declared-state-title">
                    <div className="border border-border p-5">
                        <p className="ck-label">Declared and reported state</p>
                        <h2 className="mt-2 text-xl font-semibold" id="declared-state-title">Reconciliation record</h2>
                    </div>
                    <dl className="grid gap-px border-x border-b border-border bg-border sm:grid-cols-2 xl:grid-cols-4">
                        <div className="bg-background p-4">
                            <dt className="ck-label">Readiness</dt>
                            <dd className="mt-2 font-mono text-sm">{service.ready ? "⊢ Ready reported" : "○ Not ready reported"}</dd>
                        </div>
                        <div className="bg-background p-4">
                            <dt className="ck-label">Replicas</dt>
                            <dd className="mt-2 font-mono text-sm tabular-nums">{service.replicas} current / {service.desiredReplicas} desired</dd>
                        </div>
                        <div className="bg-background p-4">
                            <dt className="ck-label">Generation</dt>
                            <dd className="mt-2 font-mono text-sm tabular-nums">{service.observedGeneration ?? "—"} observed / {service.generation ?? "—"} declared</dd>
                        </div>
                        <div className="bg-background p-4">
                            <dt className="ck-label">Autoscale ceiling</dt>
                            <dd className="mt-2 font-mono text-sm tabular-nums">{service.maxReplicas}</dd>
                        </div>
                        <div className="bg-background p-4 sm:col-span-2">
                            <dt className="ck-label">Model source</dt>
                            <dd className="mt-2 break-all font-mono text-xs">{service.uri}</dd>
                        </div>
                        <div className="bg-background p-4">
                            <dt className="ck-label">Model revision</dt>
                            <dd className="mt-2 break-all font-mono text-xs">{service.modelRevision}</dd>
                        </div>
                        <div className="bg-background p-4">
                            <dt className="ck-label">Endpoint</dt>
                            <dd className="mt-2">
                                {service.url ? (
                                    <a className="ck-text-action" href={service.url} rel="noreferrer" target="_blank">Open reported endpoint</a>
                                ) : <span className="font-mono text-xs text-muted-foreground">Not reported</span>}
                            </dd>
                        </div>
                    </dl>

                    <div className="mt-8 border border-border">
                        <div className="border-b border-border bg-muted p-4"><h3 className="text-base font-semibold">Status conditions</h3></div>
                        {service.conditions.length === 0 ? (
                            <p className="p-5 text-sm text-muted-foreground">No status conditions were returned.</p>
                        ) : service.conditions.map((condition) => (
                            <article className="grid gap-3 border-b border-border p-5 last:border-b-0 md:grid-cols-[180px_100px_minmax(0,1fr)]" key={`${condition.type}-${condition.lastTransitionTime}`}>
                                <div>
                                    <p className="font-mono text-sm font-semibold">{condition.type}</p>
                                    <p className="mt-1 font-mono text-[11px] text-muted-foreground">{condition.reason}</p>
                                </div>
                                <p className="font-mono text-xs">{condition.status}</p>
                                <div>
                                    <p className="text-sm leading-6">{condition.message}</p>
                                    <p className="mt-1 font-mono text-[11px] text-muted-foreground">{absoluteTime(condition.lastTransitionTime)}</p>
                                </div>
                            </article>
                        ))}
                    </div>
                </section>
            )}

            <section className="mt-8" aria-labelledby="runtime-context-title">
                <div className="flex flex-col justify-between gap-4 border border-border p-5 sm:flex-row sm:items-end">
                    <div>
                        <p className="ck-label">Related Kubernetes objects</p>
                        <h2 className="mt-2 text-xl font-semibold" id="runtime-context-title">Runtime context</h2>
                        <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">{runtime.detail}</p>
                    </div>
                    <span className={runtimeQualified ? "ck-claim" : "ck-claim ck-claim--muted"}>
                        {runtime.state === "observed" ? "⊢ context observed" : runtime.state === "empty" ? "⊢ context empty" : runtime.state === "partial" ? "○ context partial" : "○ context unavailable"}
                    </span>
                </div>

                <div className="mt-px border border-border">
                    <div className="border-b border-border bg-muted p-4"><h3 className="text-base font-semibold">Deployments</h3></div>
                    {runtime.data.deployments.length === 0 ? (
                        <p className="p-5 text-sm text-muted-foreground">No related Deployment records were returned.</p>
                    ) : (
                        <div aria-label="Managed deployment records table" className="ck-table-scroll" role="region" tabIndex={0}>
                            <table className="w-full min-w-[720px] border-collapse text-left">
                                <thead><tr className="border-b border-border"><th className="ck-label p-4">Name</th><th className="ck-label p-4">Ready</th><th className="ck-label p-4">Available</th><th className="ck-label p-4">Updated</th><th className="ck-label p-4">Generation</th></tr></thead>
                                <tbody>{runtime.data.deployments.map((deployment) => (
                                    <tr className="border-b border-border last:border-b-0" key={deployment.name}>
                                        <td className="p-4 font-mono text-xs font-semibold">{deployment.name}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{deployment.ready} / {deployment.desired}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{deployment.available}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{deployment.updated}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{deployment.observedGeneration ?? "—"} / {deployment.generation ?? "—"}</td>
                                    </tr>
                                ))}</tbody>
                            </table>
                        </div>
                    )}
                </div>

                <div className="mt-px border border-border">
                    <div className="border-b border-border bg-muted p-4"><h3 className="text-base font-semibold">Pods</h3></div>
                    {runtime.data.pods.length === 0 ? (
                        <p className="p-5 text-sm text-muted-foreground">No related Pod records were returned.</p>
                    ) : (
                        <div aria-label="Managed pod records table" className="ck-table-scroll" role="region" tabIndex={0}>
                            <table className="w-full min-w-[760px] border-collapse text-left">
                                <thead><tr className="border-b border-border"><th className="ck-label p-4">Name</th><th className="ck-label p-4">Phase</th><th className="ck-label p-4">Containers</th><th className="ck-label p-4">Restarts</th><th className="ck-label p-4">Node</th><th className="ck-label p-4">Created</th></tr></thead>
                                <tbody>{runtime.data.pods.map((pod) => (
                                    <tr className="border-b border-border last:border-b-0" key={pod.name}>
                                        <td className="p-4 font-mono text-xs font-semibold">{pod.name}</td>
                                        <td className="p-4 font-mono text-xs">{pod.phase}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{pod.readyContainers} / {pod.totalContainers}</td>
                                        <td className="p-4 font-mono text-xs tabular-nums">{pod.restarts}</td>
                                        <td className="p-4 font-mono text-xs">{pod.node}</td>
                                        <td className="p-4 font-mono text-xs text-muted-foreground">{absoluteTime(pod.createdAt)}</td>
                                    </tr>
                                ))}</tbody>
                            </table>
                        </div>
                    )}
                </div>

                <div className="mt-px border border-border">
                    <div className="border-b border-border bg-muted p-4">
                        <h3 className="text-base font-semibold">Reconciliation events</h3>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">Kubernetes Events are best-effort supplemental records with limited retention.</p>
                    </div>
                    {runtime.data.events.length === 0 ? (
                        <p className="p-5 text-sm text-muted-foreground">No directly related Event records were returned.</p>
                    ) : runtime.data.events.map((event) => (
                        <article className="grid gap-3 border-b border-border p-5 last:border-b-0 lg:grid-cols-[180px_140px_minmax(0,1fr)_100px]" key={event.id}>
                            <div>
                                <p className="font-mono text-xs font-semibold">⊢ {event.reason}</p>
                                <p className="mt-1 font-mono text-[11px] text-muted-foreground">{event.type}</p>
                            </div>
                            <div>
                                <p className="ck-label">Reporter</p>
                                <p className="mt-1 break-words font-mono text-xs">{event.reporter}</p>
                            </div>
                            <p className="text-sm leading-6">{event.message}</p>
                            <div className="font-mono text-[11px] text-muted-foreground">
                                <p>Count {event.count}</p>
                                <p className="mt-1">{absoluteTime(event.lastAt)}</p>
                            </div>
                        </article>
                    ))}
                </div>
            </section>
        </div>
    );
}
