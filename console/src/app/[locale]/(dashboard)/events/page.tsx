import { notFound } from "next/navigation";

import { LatticeAuditBlock } from "@/components/features/audit";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getAuditLogSource } from "@/lib/audit.server";
import { allowedInvestigationFilter, boundedInvestigationQuery } from "@/lib/investigation-query";
import { getDeploymentProfile } from "@/lib/kserve";

export const dynamic = "force-dynamic";

interface EventsPageProps {
    searchParams: Promise<{ outcome?: string | string[]; q?: string | string[] }>;
}

export default async function EventsPage({ searchParams }: EventsPageProps) {
    const routeSearch = await searchParams;
    const initialQuery = boundedInvestigationQuery(routeSearch.q);
    const initialOutcome = allowedInvestigationFilter(routeSearch.outcome, ["all", "success", "failure", "denied"] as const, "all");
    const [audit, profile] = await Promise.all([
        getAuditLogSource(250),
        getDeploymentProfile(),
    ]);

    if (!profile.features.audit) notFound();

    const sourceAvailable = audit.state !== "unavailable";
    const deniedOrFailed = audit.data.filter((event) => event.outcome !== "Success").length;
    const actors = new Set(audit.data.map((event) => event.actor)).size;

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Evidence boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        This ledger displays validated runtime records from the configured JSONL source. It does not assert signature verification or retention compliance.
                    </p>
                </div>
            )}
            description="Search the operator's runtime audit contract by actor, action, outcome, resource, execution identity, and structured context."
            eyebrow="Governance / runtime record"
            metrics={[
                { label: "Parsed", value: sourceAvailable ? audit.data.length : "—" },
                { label: "Denied or failed", value: sourceAvailable ? deniedOrFailed : "—" },
                { label: "Actors", value: sourceAvailable ? actors : "—" },
            ]}
            profile={profile}
            sources={[{ label: "audit", snapshot: audit }]}
            title="Audit event ledger"
        >
            <section className="ck-quiet" aria-label="Audit event records">
                <LatticeAuditBlock audit={audit} initialOutcome={initialOutcome} initialQuery={initialQuery} />
            </section>
        </OperatorPage>
    );
}
