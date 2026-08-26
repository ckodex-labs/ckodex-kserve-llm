import { getInferenceInventory, getLatticeHealthSource, getDeploymentProfile } from "@/lib/kserve";
import { getAuditLogSource } from "@/lib/audit.server";
import { DashboardEvidenceContext, DashboardView } from "@/components/features/DashboardView";
import { LivePoller } from "@/components/features/LivePoller";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getDashboardAttention, isInventoryQualified } from "@/components/features/dashboard-state";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const [inventory, health, profile, audit] = await Promise.all([
    getInferenceInventory(),
    getLatticeHealthSource(),
    getDeploymentProfile(),
    getAuditLogSource(15),
  ]);

  const inventoryQualified = isInventoryQualified(inventory);
  const ready = inventory.data.filter((service) => service.ready).length;
  const attention = getDashboardAttention(inventory, health, audit).length;

  return (
    <OperatorPage
      aside={<DashboardEvidenceContext audit={audit} showAudit={profile.features.audit} />}
      description={`Desired model-serving resources, observed Kubernetes state, and evidence qualification for ${profile.latticeName}.`}
      eyebrow="Model serving / reconciliation"
      metrics={[
        { label: "Declared", value: inventoryQualified ? inventory.data.length : "—" },
        { label: "Ready", value: inventoryQualified ? ready : "—" },
        { label: "Attention", value: attention },
      ]}
      profile={profile}
      sources={[
        { label: "inventory", snapshot: inventory },
        { label: "namespace checks", snapshot: health },
        { label: "audit", snapshot: audit },
      ]}
      title="Reconciliation overview"
      toolbarActions={<LivePoller />}
    >
      <DashboardView
        inventory={inventory}
        health={health}
        audit={audit}
        showAudit={profile.features.audit}
      />
    </OperatorPage>
  );
}
