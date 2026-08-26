"use client";

import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";

import type { DeploymentProfile } from "@/lib/config";
import { formatObservationTime, sourceClaim, type SourceSnapshot } from "@/lib/source";

export interface OperatorSource {
    label: string;
    snapshot: Pick<SourceSnapshot<unknown>, "state" | "source" | "observedAt" | "detail">;
}

const evidenceMarginStorageKey = "ck-evidence-margin";

function EvidenceMargin({
    sources,
    aside,
    collapsed,
    onCollapsedChange,
}: {
    sources: OperatorSource[];
    aside?: ReactNode;
    collapsed: boolean;
    onCollapsedChange: (collapsed: boolean) => void;
}) {
    const [copyStatus, setCopyStatus] = useState<{ source: string; state: "copied" | "failed" } | null>(null);
    const summary = useMemo(() => {
        const qualified = sources.filter(({ snapshot }) => snapshot.state === "observed" || snapshot.state === "empty").length;
        return { qualified, unresolved: sources.length - qualified };
    }, [sources]);

    async function copySource(source: string) {
        try {
            await navigator.clipboard.writeText(source);
            setCopyStatus({ source, state: "copied" });
        } catch {
            setCopyStatus({ source, state: "failed" });
        }

        window.setTimeout(() => setCopyStatus((current) => current?.source === source ? null : current), 1600);
    }

    return (
        <aside className="ck-evidence-margin" aria-labelledby="operator-evidence-title">
            <div className="ck-evidence-margin__rail" aria-hidden={!collapsed}>
                <button
                    aria-label="Expand evidence margin"
                    className="ck-evidence-margin__rail-button"
                    onClick={() => onCollapsedChange(false)}
                    type="button"
                >
                    <span aria-hidden="true">◐</span>
                    <span className="ck-evidence-margin__rail-label">Evidence margin</span>
                    <span className="ck-evidence-margin__rail-count">{sources.length}</span>
                </button>
            </div>

            <div className="ck-evidence-margin__body">
                <div className="flex items-start justify-between gap-4">
                    <div>
                        <p className="ck-label">Source ledger</p>
                        <h2 id="operator-evidence-title" className="mt-2 text-base font-semibold">Evidence margin</h2>
                    </div>
                    <button
                        aria-expanded={!collapsed}
                        aria-label="Collapse evidence margin"
                        className="ck-evidence-margin__toggle"
                        onClick={() => onCollapsedChange(true)}
                        type="button"
                    >
                        <span aria-hidden="true">◐</span>
                    </button>
                </div>

                <p className="mt-3 text-sm leading-6 text-muted-foreground">
                    Source observations qualify this view. No cryptographic proof object is attached.
                </p>

                <dl className="ck-evidence-summary mt-5">
                    <div>
                        <dt className="ck-label">Qualified</dt>
                        <dd>{summary.qualified}</dd>
                    </div>
                    <div>
                        <dt className="ck-label">Unresolved</dt>
                        <dd>{summary.unresolved}</dd>
                    </div>
                    <div>
                        <dt className="ck-label">Seal</dt>
                        <dd className="text-sm">○ absent</dd>
                    </div>
                </dl>

                <div className="mt-6">
                    {sources.map(({ label, snapshot }) => {
                        const copyState = copyStatus?.source === snapshot.source
                            ? copyStatus.state === "copied" ? "Source copied" : "Clipboard unavailable"
                            : null;

                        return (
                            <article className="ck-receipt" key={`${label}-${snapshot.source}`}>
                                <span className="ck-receipt__rule" aria-hidden="true" />
                                <div className="min-w-0">
                                    <p className="ck-receipt__event mt-0">{sourceClaim(label, snapshot.state)}</p>
                                    <p className="ck-receipt__time mt-1">{formatObservationTime(snapshot.observedAt)}</p>
                                    <p className="ck-receipt__detail">{snapshot.source}</p>

                                    <details className="ck-source-inspector">
                                        <summary>Inspect source boundary</summary>
                                        <p className="ck-receipt__detail mt-3">{snapshot.detail}</p>
                                        <button
                                            className="ck-source-copy"
                                            onClick={() => void copySource(snapshot.source)}
                                            type="button"
                                        >
                                            {copyState ?? "Copy source reference"}
                                        </button>
                                    </details>
                                </div>
                            </article>
                        );
                    })}
                </div>

                {aside ? <div className="mt-8 border-t border-border pt-7">{aside}</div> : null}

                <div className="mt-8 border border-dashed border-border p-4">
                    <p className="ck-label">Qualification</p>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        Observed data is not attested data. Violet is withheld until a verifiable evidence envelope resolves.
                    </p>
                </div>
            </div>
        </aside>
    );
}

export function OperatorShell({
    profile,
    sources,
    aside,
    children,
}: {
    profile: DeploymentProfile;
    sources: OperatorSource[];
    aside?: ReactNode;
    children: ReactNode;
}) {
    const [collapsed, setCollapsed] = useState(false);
    const [restored, setRestored] = useState(false);

    useEffect(() => {
        const stored = window.localStorage.getItem(evidenceMarginStorageKey);
        const frame = window.requestAnimationFrame(() => {
            setCollapsed(stored === "collapsed");
            setRestored(true);
        });
        return () => window.cancelAnimationFrame(frame);
    }, []);

    useEffect(() => {
        if (!restored) return;
        window.localStorage.setItem(evidenceMarginStorageKey, collapsed ? "collapsed" : "expanded");
        document.documentElement.dataset.evidenceMargin = collapsed ? "collapsed" : "expanded";
        return () => { delete document.documentElement.dataset.evidenceMargin; };
    }, [collapsed, restored]);

    return (
        <div className="ck-dashboard-shell" data-evidence-collapsed={collapsed ? "true" : "false"}>
            {children}

            <EvidenceMargin
                aside={aside}
                collapsed={collapsed}
                onCollapsedChange={setCollapsed}
                sources={sources}
            />

            <footer className="ck-authority">
                <div>
                    <span className="text-foreground">◇ Internal</span>
                    <span className="ml-3">CKodex KServe LLM Operator</span>
                </div>
                <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
                    <span>Environment: {profile.environment}</span>
                    <span>Mode: observe</span>
                    <span className="ck-invariant">mode changes deployment, not governance semantics</span>
                </div>
            </footer>
        </div>
    );
}
