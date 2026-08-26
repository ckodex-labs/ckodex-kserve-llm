"use client";

import { Button, buttonVariants } from "@/components/ui/button";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import { Link } from "@/i18n/routing";
import { ThemeControl } from "./ThemeControl";

type OperatorRouteStateKind = "loading" | "error" | "missing";

interface OperatorRouteStateProps {
    kind: OperatorRouteStateKind;
    title: string;
    description: string;
    reference?: string;
    onRetry?: () => void;
}

const stateCopy: Record<OperatorRouteStateKind, {
    eyebrow: string;
    claim: string;
    ledgerEvent: string;
    ledgerTime: string;
    source: string;
}> = {
    loading: {
        eyebrow: "Route resolution",
        claim: "○ observation pending",
        ledgerEvent: "○ route sources resolving",
        ledgerTime: "Resolution in progress",
        source: "Next.js route segment · no completed observation",
    },
    error: {
        eyebrow: "Route recovery",
        claim: "⊭ render incomplete",
        ledgerEvent: "⊭ route render interrupted",
        ledgerTime: "Recovery required",
        source: "Next.js route boundary · server sources unqualified",
    },
    missing: {
        eyebrow: "Route availability",
        claim: "○ surface unavailable",
        ledgerEvent: "○ route not resolved",
        ledgerTime: "No route observation",
        source: "Route registry or active deployment profile",
    },
};

export function OperatorRouteState({
    kind,
    title,
    description,
    reference,
    onRetry,
}: OperatorRouteStateProps) {
    const copy = stateCopy[kind];
    const isLoading = kind === "loading";
    const evidenceContent = (
        <>
            <p className="ck-label">Source ledger</p>
            <h2 className="mt-2 text-base font-semibold" id="route-evidence-title">Evidence margin</h2>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
                The route state is recorded without implying a Kubernetes observation or proof object.
            </p>
            <div className="mt-7">
                <article className="ck-receipt">
                    <span className="ck-receipt__rule" aria-hidden="true" />
                    <div className="min-w-0">
                        <p className="ck-receipt__event mt-0">{copy.ledgerEvent}</p>
                        <p className="ck-receipt__time mt-1">{copy.ledgerTime}</p>
                        <p className="ck-receipt__detail">{copy.source}</p>
                        {reference ? <p className="ck-route-reference">Runtime reference · {reference}</p> : null}
                    </div>
                </article>
            </div>
            <div className="mt-8 border border-dashed border-border p-4">
                <p className="ck-label">Qualification</p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.claim} · evidence unsealed</p>
            </div>
        </>
    );
    const authorityContent = (
        <>
            <div><span className="text-foreground">◇ Internal</span><span className="ml-3">Recovery plane</span></div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
                <span>Mode: observe</span>
                <span className="ck-invariant">mode changes deployment, not governance semantics</span>
            </div>
        </>
    );

    const content = (
        <>
            <header className="sticky top-0 z-50 flex min-h-16 items-center gap-3 border-b border-border bg-background px-4 py-2">
                <SidebarTrigger className="-ml-1" />
                <div className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold tracking-tight">CKodex KServe LLM Operator</span>
                    <span className="mt-0.5 block font-mono text-[11px] uppercase tracking-[0.1em] text-muted-foreground">
                        Recovery plane · observe
                    </span>
                </div>
                <ThemeControl />
            </header>

            <div className="ck-route-shell">
                <section
                    aria-busy={isLoading || undefined}
                    aria-live={kind === "error" ? "assertive" : "polite"}
                    className="ck-route-state"
                    data-route-state={kind}
                    role={isLoading ? "status" : kind === "error" ? "alert" : undefined}
                >
                    <div className="ck-route-state__mark" aria-hidden="true">{kind === "error" ? "⊭" : "○"}</div>
                    <p className="ck-label mt-8">{copy.eyebrow}</p>
                    {isLoading ? (
                        <div aria-level={1} className="mt-4 max-w-3xl font-heading text-4xl leading-[1.05] tracking-[-0.035em] text-foreground md:text-6xl" role="heading">
                            {title}
                        </div>
                    ) : (
                        <h1 className="mt-4 max-w-3xl font-heading text-4xl leading-[1.05] tracking-[-0.035em] text-foreground md:text-6xl">
                            {title}
                        </h1>
                    )}
                    <p className="mt-5 max-w-[64ch] text-base leading-7 text-muted-foreground">{description}</p>

                    {isLoading ? (
                        <div className="ck-route-resolve mt-9" aria-hidden="true"><span /></div>
                    ) : (
                        <div className="mt-9 flex flex-wrap gap-3">
                            {onRetry ? <Button onClick={onRetry}>Retry route</Button> : null}
                            <Link className={buttonVariants({ variant: onRetry ? "outline" : "default" })} href="/">
                                Reconciliation overview
                            </Link>
                        </div>
                    )}

                    <div className="ck-quiet mt-10 max-w-3xl p-5">
                        <p className="ck-label">Recovery contract</p>
                        <p className="mt-3 text-sm leading-6 text-muted-foreground">
                            {isLoading
                                ? "No source, count, or readiness assertion is emitted until the route settles."
                                : "Navigation remains available. Source qualification resumes only after a successful route render."}
                        </p>
                    </div>
                </section>

                {isLoading ? (
                    <div aria-labelledby="route-evidence-title" className="ck-route-evidence" role="complementary">{evidenceContent}</div>
                ) : (
                    <aside aria-labelledby="route-evidence-title" className="ck-route-evidence">{evidenceContent}</aside>
                )}

                {isLoading ? (
                    <div className="ck-authority" role="contentinfo">{authorityContent}</div>
                ) : (
                    <footer className="ck-authority">{authorityContent}</footer>
                )}
            </div>
        </>
    );

    if (isLoading) {
        return (
            <div className="ck-operator-inset relative flex w-full flex-1 flex-col bg-background" id="ck-main" role="main" tabIndex={-1}>
                {content}
            </div>
        );
    }

    return <SidebarInset className="ck-operator-inset" id="ck-main" tabIndex={-1}>{content}</SidebarInset>;
}
