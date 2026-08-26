import { notFound } from "next/navigation";

import { FabricTerminalBlock } from "@/components/features/terminal";
import { OperatorPage } from "@/components/layout/OperatorPage";
import { getDeploymentProfile } from "@/lib/kserve";
import type { SourceSnapshot } from "@/lib/source";

export const dynamic = "force-dynamic";

const executionSource: SourceSnapshot<never[]> = {
    state: "unavailable",
    data: [],
    source: "Console command-execution adapter",
    observedAt: null,
    detail: "No authenticated command-execution adapter is configured. The page provides copyable command references only.",
};

export default async function TerminalPage() {
    const profile = await getDeploymentProfile();

    if (!profile.features.terminal) notFound();

    return (
        <OperatorPage
            aside={(
                <div className="border border-border p-4">
                    <p className="ck-label">Execution boundary</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        A future execution path requires authenticated identity, scoped authorization, command allowlisting, confirmation, and an audit receipt.
                    </p>
                </div>
            )}
            description="A reviewed command reference for local operator workflows. Cluster execution is explicitly unavailable."
            eyebrow="Operations / command reference"
            metrics={[
                { label: "Commands", value: 4 },
                { label: "Executed", value: "—" },
            ]}
            profile={profile}
            sources={[{ label: "execution", snapshot: executionSource }]}
            title="Command library"
        >
            <FabricTerminalBlock />
        </OperatorPage>
    );
}
