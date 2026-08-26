import { notFound } from "next/navigation";

import { IdentityAuthorityBlock } from "@/components/features/identity";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getAccessReviewSource, getIdentityRegistrationSource } from "@/lib/identity.server";
import { getDeploymentProfile } from "@/lib/kserve";

export const dynamic = "force-dynamic";

export default async function IdentityAuthorityPage() {
    const [registrations, access, profile] = await Promise.all([
        getIdentityRegistrationSource(),
        getAccessReviewSource(),
        getDeploymentProfile(),
    ]);

    if (!profile.features.identity) notFound();

    const allowedObservations = access.data.capabilities.filter((item) => item.effect === "observe" && item.decision === "allowed").length;
    const mutationGrants = access.data.capabilities.filter((item) => item.effect === "mutate" && item.decision === "allowed").length;

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Authority boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Registration is not issuance. Service-principal access is not human authorization. Mutation remains outside this observe-only surface.
                    </p>
                </div>
            )}
            description="SPIRE registration intent and current Kubernetes authority, separated by source and assertion strength."
            eyebrow="Governance / identity and authority"
            metrics={[
                { label: "Registrations", value: registrations.data.length },
                { label: "Observed grants", value: allowedObservations },
                { label: "Mutation grants", value: mutationGrants },
            ]}
            profile={profile}
            sources={[
                { label: "registration", snapshot: registrations },
                { label: "access review", snapshot: access },
            ]}
            title="Identity and authority"
        >
            <IdentityAuthorityBlock access={access} registrations={registrations} />
        </OperatorPage>
    );
}
