import { NodeManagerBlock } from "@/components/features/infrastructure/NodeManagerBlock";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getDeploymentProfile, getNodeSource } from "@/lib/kserve";
import { isSourceObserved } from "@/lib/source";

export const dynamic = "force-dynamic";

export default async function NodesPage() {
    const [nodes, profile] = await Promise.all([
        getNodeSource(),
        getDeploymentProfile(),
    ]);
    const qualified = isSourceObserved(nodes);
    const ready = nodes.data.filter((node) => node.status === "Ready").length;

    return (
        <OperatorPage
            description="Kubernetes node readiness, topology labels, and reported resource capacity in the active context."
            eyebrow="Infrastructure / cluster"
            metrics={[
                { label: "Observed", value: qualified ? nodes.data.length : "—" },
                { label: "Ready", value: qualified ? ready : "—" },
                { label: "Attention", value: qualified ? nodes.data.length - ready : "—" },
            ]}
            profile={profile}
            sources={[{ label: "nodes", snapshot: nodes }]}
            title="Node inventory"
        >
            <NodeManagerBlock source={nodes} />
        </OperatorPage>
    );
}
