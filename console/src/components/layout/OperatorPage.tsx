import type { ReactNode } from "react";

import type { DeploymentProfile } from "@/lib/config";
import { isSourceObserved, sourceClaim } from "@/lib/source";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import { RefreshControl } from "./RefreshControl";
import { ThemeControl } from "./ThemeControl";
import { OperatorShell, type OperatorSource } from "./OperatorShell";
import { OperatorCommandPalette } from "./OperatorCommandPalette";

export interface OperatorMetric {
    label: string;
    value: ReactNode;
}

interface OperatorPageProps {
    profile: DeploymentProfile;
    eyebrow: string;
    title: string;
    description: string;
    sources: OperatorSource[];
    metrics?: OperatorMetric[];
    children: ReactNode;
    aside?: ReactNode;
    toolbarActions?: ReactNode;
}

export function OperatorToolbar({ profile, actions }: { profile: DeploymentProfile; actions?: ReactNode }) {
    return (
        <header className="sticky top-0 z-50 flex min-h-16 shrink-0 flex-wrap items-center gap-2 border-b border-border bg-background px-4 py-2">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 hidden h-5 border-l border-border sm:block" />
            <div className="min-w-0 flex-1 px-1">
                <span className="block truncate text-sm font-semibold tracking-tight">{profile.latticeName}</span>
                <span className="mt-0.5 block font-mono text-[11px] uppercase tracking-[0.1em] text-muted-foreground sm:hidden">
                    {profile.environment} · observe
                </span>
            </div>
            <div className="flex items-center gap-2">
                <OperatorCommandPalette profile={profile} />
                {actions || <RefreshControl />}
                <ThemeControl />
            </div>
        </header>
    );
}

export function OperatorPage({
    profile,
    eyebrow,
    title,
    description,
    sources,
    metrics = [],
    children,
    aside,
    toolbarActions,
}: OperatorPageProps) {
    return (
            <SidebarInset className="ck-operator-inset" id="ck-main" tabIndex={-1}>
            <OperatorToolbar actions={toolbarActions} profile={profile} />
            <OperatorShell aside={aside} profile={profile} sources={sources}>
                <div className="ck-dashboard-main" id="ck-page-content">
                    <header className="border-b border-border pb-8">
                        <p className="ck-label">{eyebrow}</p>
                        <div className="ck-page-hero mt-3">
                            <div className="max-w-3xl">
                                <h1 className="font-heading text-4xl leading-[1.05] tracking-[-0.035em] text-foreground md:text-6xl">
                                    {title}
                                </h1>
                                <p className="mt-5 max-w-[72ch] text-base leading-7 text-muted-foreground">{description}</p>
                            </div>
                            {metrics.length > 0 ? (
                                <dl className="ck-page-metrics grid border border-border" style={{ gridTemplateColumns: `repeat(${metrics.length}, minmax(0, 1fr))` }}>
                                    {metrics.map((metric, index) => (
                                        <div className={index > 0 ? "border-l border-border p-4" : "p-4"} key={metric.label}>
                                            <dt className="ck-label">{metric.label}</dt>
                                            <dd className="mt-2 font-mono text-2xl font-semibold tabular-nums">{metric.value}</dd>
                                        </div>
                                    ))}
                                </dl>
                            ) : null}
                        </div>
                        <div className="mt-7 flex flex-wrap items-center gap-2">
                            {sources.map(({ label, snapshot }) => (
                                <span className={isSourceObserved(snapshot) ? "ck-claim" : "ck-claim ck-claim--muted"} key={`${label}-${snapshot.source}`}>
                                    {sourceClaim(label, snapshot.state)}
                                </span>
                            ))}
                            <span className="ck-claim ck-claim--muted">○ evidence unsealed</span>
                            <span className="ck-claim ck-claim--muted">{profile.environment}</span>
                        </div>
                    </header>

                    <div className="mt-10">{children}</div>
                </div>
            </OperatorShell>
        </SidebarInset>
    );
}
