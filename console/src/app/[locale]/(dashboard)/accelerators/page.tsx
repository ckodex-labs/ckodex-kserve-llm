import { AcceleratorFleetBlock } from "@/components/features/infrastructure/AcceleratorFleetBlock";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getAcceleratorSource, getDeploymentProfile } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";

export const dynamic = "force-dynamic";

export default async function AcceleratorsPage() {
    const [accelerators, profile] = await Promise.all([
        getAcceleratorSource(),
        getDeploymentProfile(),
    ]);
    const qualified = isSourceObserved(accelerators);
    const nodeCount = new Set(accelerators.data.map((accelerator) => accelerator.node)).size;

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Metric boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Utilization requires a metrics adapter such as DCGM. Kubernetes allocatable capacity alone cannot substantiate load or memory telemetry.
                    </p>
                </div>
            )}
            description="Accelerator extended resources reported as allocatable by Kubernetes nodes, without inferred device telemetry."
            eyebrow="Infrastructure / accelerators"
            metrics={[
                { label: "Allocations", value: qualified ? accelerators.data.length : "—" },
                { label: "Nodes", value: qualified ? nodeCount : "—" },
            ]}
            profile={profile}
            sources={[{ label: "capacity", snapshot: accelerators }]}
            title="Accelerator inventory"
        >
            <AcceleratorFleetBlock source={accelerators} />
        </OperatorPage>
    );
}
