import { notFound } from "next/navigation";

import { TelemetryView } from "@/components/features/telemetry/TelemetryView";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { allowedInvestigationFilter, boundedInvestigationQuery } from "@/lib/investigation-query";
import { getDeploymentProfile } from "@/lib/kserve";
import { safeTelemetryLink } from "@/lib/telemetry";
import { getTelemetrySource } from "@/lib/telemetry.server";

export const dynamic = "force-dynamic";

interface TelemetryPageProps {
    searchParams: Promise<{ q?: string | string[]; state?: string | string[] }>;
}

export default async function TelemetryPage({ searchParams }: TelemetryPageProps) {
    const routeSearch = await searchParams;
    const initialQuery = boundedInvestigationQuery(routeSearch.q);
    const initialState = allowedInvestigationFilter(routeSearch.state, ["all", "firing", "pending"] as const, "all");
    const [telemetry, profile] = await Promise.all([
        getTelemetrySource(),
        getDeploymentProfile(),
    ]);

    if (!profile.features.telemetry) notFound();

    const sourceAvailable = telemetry.state !== "unavailable";
    const firing = telemetry.data.alerts.filter((alert) => alert.state.toLowerCase() === "firing").length;
    const grafanaDashboardUrl = safeTelemetryLink(process.env.CKODEX_GRAFANA_DASHBOARD_URL);

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Measurement boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Metrics and alerts are observed Prometheus data. They are not cryptographic proof, incident acknowledgement, or an emergency-protocol assertion.
                    </p>
                    {grafanaDashboardUrl ? (
                        <a className="ck-text-action mt-4" href={grafanaDashboardUrl} rel="noreferrer" target="_blank">
                            Open detailed Grafana dashboard
                        </a>
                    ) : null}
                </div>
            )}
            description="Allowlisted CKODEX and vLLM signals, measurement trends, and active Prometheus alerts for operator triage."
            eyebrow="Operations / measurement"
            metrics={[
                { label: "Series", value: sourceAvailable ? telemetry.data.series.length : "—" },
                { label: "Firing alerts", value: sourceAvailable ? firing : "—" },
                { label: "Source warnings", value: sourceAvailable ? telemetry.data.warnings.length : "—" },
            ]}
            profile={profile}
            sources={[{ label: "telemetry", snapshot: telemetry }]}
            title="Runtime telemetry"
        >
            <TelemetryView initialQuery={initialQuery} initialState={initialState} source={telemetry} />
        </OperatorPage>
    );
}
