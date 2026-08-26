import type { AcceleratorSource } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";
import { SourceBoundaryPanel } from "@/components/layout/SourceBoundaryPanel";

export function AcceleratorFleetBlock({ source }: { source: AcceleratorSource }) {
    if (!isSourceObserved(source)) {
        return <SourceBoundaryPanel label="accelerators" snapshot={source} title="Accelerator inventory unavailable" />;
    }

    return (
        <section aria-labelledby="accelerator-inventory-title">
            <div className="border border-border p-5">
                <p className="ck-label">Allocatable resources</p>
                <h2 className="mt-2 text-xl font-semibold" id="accelerator-inventory-title">Node accelerator capacity</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    Extended resources reported by Kubernetes. Device model, utilization, memory, and workload assignment are withheld because this source does not provide them.
                </p>
            </div>

            <div aria-label="Accelerator inventory table" className="ck-table-scroll mt-px border border-border" role="region" tabIndex={0}>
                <table className="w-full min-w-[620px] border-collapse text-left">
                    <thead className="bg-muted">
                        <tr>
                            <th className="ck-label p-4" scope="col">Node</th>
                            <th className="ck-label p-4" scope="col">Resource key</th>
                            <th className="ck-label p-4" scope="col">Class</th>
                            <th className="ck-label p-4 text-right" scope="col">Allocatable</th>
                        </tr>
                    </thead>
                    <tbody>
                        {source.data.length === 0 ? (
                            <tr>
                                <td className="border-t border-border p-8 text-center text-sm text-muted-foreground" colSpan={4}>
                                    No supported accelerator resources were reported as allocatable.
                                </td>
                            </tr>
                        ) : source.data.map((accelerator) => (
                            <tr className="border-t border-border" key={accelerator.id}>
                                <td className="p-4 font-mono text-sm font-semibold">{accelerator.node}</td>
                                <td className="p-4 font-mono text-xs text-muted-foreground">{accelerator.resourceName}</td>
                                <td className="p-4"><span className="ck-claim">⊢ {accelerator.type}</span></td>
                                <td className="p-4 text-right font-mono text-sm tabular-nums">{accelerator.allocatable}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </section>
    );
}
