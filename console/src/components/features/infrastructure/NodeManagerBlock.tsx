import type { NodeSource } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";
import { SourceBoundaryPanel } from "@/components/layout/SourceBoundaryPanel";

export function NodeManagerBlock({ source }: { source: NodeSource }) {
    if (!isSourceObserved(source)) {
        return <SourceBoundaryPanel label="nodes" snapshot={source} title="Node inventory unavailable" />;
    }

    return (
        <section aria-labelledby="node-inventory-title">
            <div className="border border-border p-5">
                <p className="ck-label">Observed capacity</p>
                <h2 className="mt-2 text-xl font-semibold" id="node-inventory-title">Cluster nodes</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    Readiness, role labels, architecture, and reported capacity from Kubernetes Node resources.
                </p>
            </div>

            <div aria-label="Node inventory table" className="ck-table-scroll mt-px border border-border" role="region" tabIndex={0}>
                <table className="w-full min-w-[720px] border-collapse text-left">
                    <thead className="bg-muted">
                        <tr>
                            <th className="ck-label p-4" scope="col">Node</th>
                            <th className="ck-label p-4" scope="col">Readiness</th>
                            <th className="ck-label p-4" scope="col">Role</th>
                            <th className="ck-label p-4" scope="col">CPU capacity</th>
                            <th className="ck-label p-4" scope="col">Memory capacity</th>
                            <th className="ck-label p-4 text-right" scope="col">Age</th>
                        </tr>
                    </thead>
                    <tbody>
                        {source.data.length === 0 ? (
                            <tr>
                                <td className="border-t border-border p-8 text-center text-sm text-muted-foreground" colSpan={6}>
                                    The active context returned no Node resources.
                                </td>
                            </tr>
                        ) : source.data.map((node) => (
                            <tr className="border-t border-border" key={node.name}>
                                <td className="p-4">
                                    <p className="font-mono text-sm font-semibold">{node.name}</p>
                                    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">{node.architecture} / {node.os}</p>
                                </td>
                                <td className="p-4">
                                    <span className={node.status === "Ready" ? "ck-claim" : "ck-claim ck-claim--muted"}>
                                        {node.status === "Ready" ? "⊢ ready" : "○ not ready"}
                                    </span>
                                </td>
                                <td className="p-4 font-mono text-xs text-muted-foreground">{node.role}</td>
                                <td className="p-4 font-mono text-sm tabular-nums">{node.cpu}</td>
                                <td className="p-4 font-mono text-sm tabular-nums">{node.memory}</td>
                                <td className="p-4 text-right font-mono text-xs text-muted-foreground">{node.age}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </section>
    );
}
