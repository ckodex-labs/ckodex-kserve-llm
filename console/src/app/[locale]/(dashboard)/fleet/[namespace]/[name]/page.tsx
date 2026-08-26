import { notFound } from "next/navigation";

import { WorkloadDetail } from "@/components/features/fleet/WorkloadDetail";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getDeploymentProfile, getInferenceInventory } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";
import { getWorkloadRuntimeSource } from "@/lib/workload.server";

export const dynamic = "force-dynamic";

interface WorkloadPageProps {
    params: Promise<{ namespace: string; name: string }>;
}

export default async function WorkloadPage({ params }: WorkloadPageProps) {
    const { namespace, name } = await params;
    const [inventory, runtime, profile] = await Promise.all([
        getInferenceInventory(),
        getWorkloadRuntimeSource(namespace, name),
        getDeploymentProfile(),
    ]);

    if (!profile.features.fleet) notFound();

    const inventoryQualified = isSourceObserved(inventory);
    const service = inventory.data.find((item) => item.namespace === namespace && item.name === name) || null;
    if (inventoryQualified && !service) notFound();

    const runtimeAvailable = runtime.state !== "unavailable";
    const readyPods = runtime.data.pods.filter((pod) => pod.totalContainers > 0 && pod.readyContainers === pod.totalContainers).length;

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Control boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Observe-only. Desired-state changes remain in reviewed manifests and the operator reconciliation path.
                    </p>
                </div>
            )}
            description="Declared model-serving intent, reported conditions, managed Deployments and Pods, and retention-limited Kubernetes Events."
            eyebrow={`Model serving / ns/${namespace}`}
            metrics={[
                { label: "Ready", value: service ? service.ready ? "Yes" : "No" : "—" },
                { label: "Pods ready", value: runtimeAvailable ? `${readyPods} / ${runtime.data.pods.length}` : "—" },
                { label: "Events", value: runtimeAvailable ? runtime.data.events.length : "—" },
            ]}
            profile={profile}
            sources={[
                { label: "inventory", snapshot: inventory },
                { label: "runtime context", snapshot: runtime },
            ]}
            title={name}
        >
            <WorkloadDetail runtime={runtime} service={service} />
        </OperatorPage>
    );
}
