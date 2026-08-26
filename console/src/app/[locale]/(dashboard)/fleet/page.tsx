import { notFound } from "next/navigation";

import { FleetViewBlock } from "@/components/features/fleet";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { allowedInvestigationFilter, boundedInvestigationQuery } from "@/lib/investigation-query";
import { getDeploymentProfile, getInferenceInventory } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";

export const dynamic = "force-dynamic";

interface FleetPageProps {
    searchParams: Promise<{ q?: string | string[]; readiness?: string | string[] }>;
}

export default async function FleetPage({ searchParams }: FleetPageProps) {
    const routeSearch = await searchParams;
    const initialQuery = boundedInvestigationQuery(routeSearch.q);
    const initialReadiness = allowedInvestigationFilter(routeSearch.readiness, ["all", "ready", "attention"] as const, "all");
    const [inventory, profile] = await Promise.all([
        getInferenceInventory(),
        getDeploymentProfile(),
    ]);

    if (!profile.features.fleet) notFound();

    const qualified = isSourceObserved(inventory);
    const ready = inventory.data.filter((service) => service.ready).length;
    const attention = inventory.data.filter((service) => (
        !service.ready
        || service.generation === null
        || service.observedGeneration === null
        || service.generation !== service.observedGeneration
    )).length;

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Control boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Observe-only. Change workloads through reviewed Kubernetes manifests and the operator reconciliation path.
                    </p>
                </div>
            )}
            description="Declared model-serving resources, their reported readiness, replica envelope, and governance state planes."
            eyebrow="Model serving / inventory"
            metrics={[
                { label: "Declared", value: qualified ? inventory.data.length : "—" },
                { label: "Ready", value: qualified ? ready : "—" },
                { label: "Attention", value: qualified ? attention : "—" },
            ]}
            profile={profile}
            sources={[{ label: "inventory", snapshot: inventory }]}
            title="Inference fleet"
        >
            <FleetViewBlock initialQuery={initialQuery} initialReadiness={initialReadiness} inventory={inventory} />
        </OperatorPage>
    );
}
