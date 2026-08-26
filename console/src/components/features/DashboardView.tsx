import { DashboardInventoryBlock } from "./inventory";
import { DashboardStatusBlock } from "./status";
import { LatticeAuditBlock } from "./audit";
import type { InferenceInventorySource, LatticeHealthSource } from "@/lib/kserve";
import type { AuditLogSource } from "@/lib/audit.server";
import { auditEventGlyph, auditEventSummary } from "@/lib/audit";

interface DashboardViewProps {
    inventory: InferenceInventorySource;
    health: LatticeHealthSource;
    audit: AuditLogSource;
    showAudit: boolean;
}

function formatObservedAt(value: string | null) {
    if (!value) return "No successful observation";

    return new Date(value).toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    });
}

export function DashboardEvidenceContext({ audit, showAudit }: { audit: AuditLogSource; showAudit: boolean }) {
    const latest = audit.data[0];
    const sourceClaim = audit.state === "observed"
        ? "⊢ source observed"
        : audit.state === "empty"
            ? "⊢ empty source observed"
            : audit.state === "malformed"
                ? "○ source partial"
                : "○ source unavailable";

    return (
        <div>
            <p className="ck-label">Latest valid audit record</p>
            <div className="mt-3">
                <div className="ck-receipt">
                    <span className="ck-receipt__rule" aria-hidden="true" />
                    <div>
                        <p className="ck-receipt__time">{formatObservedAt(audit.observedAt)}</p>
                        <p className="ck-receipt__event">{sourceClaim}</p>
                        <p className="ck-receipt__detail">{audit.source}</p>
                        <p className="ck-receipt__detail">{audit.detail}</p>
                    </div>
                </div>

                {latest ? (
                    <div className="ck-receipt">
                        <span className="ck-receipt__rule" aria-hidden="true" />
                        <div>
                            <time className="ck-receipt__time" dateTime={latest.timestamp}>
                                {formatObservedAt(latest.timestamp)}
                            </time>
                            <p className="ck-receipt__event">{auditEventGlyph(latest.outcome)} {latest.action}</p>
                            <p className="ck-receipt__detail">{auditEventSummary(latest)}</p>
                            <p className="ck-receipt__detail">{latest.outcome} · {latest.actor} · {latest.resource}</p>
                        </div>
                    </div>
                ) : null}
            </div>

            {showAudit ? <a className="ck-text-action mt-5" href="#audit-records">View audit records</a> : null}
        </div>
    );
}

export function DashboardView({ inventory, health, audit, showAudit }: DashboardViewProps) {
    return (
        <div className="ck-dashboard-grid gap-px bg-border">
            <section className="ck-dashboard-status ck-quiet" aria-label="Reconciliation summary">
                <DashboardStatusBlock inventory={inventory} health={health} audit={audit} />
            </section>
            <section className="ck-dashboard-inventory ck-quiet" aria-label="Inference services">
                <DashboardInventoryBlock inventory={inventory} />
            </section>
            {showAudit ? (
                <section className="ck-dashboard-audit ck-quiet" id="audit-records" aria-label="Audit records">
                    <LatticeAuditBlock audit={audit} />
                </section>
            ) : null}
        </div>
    );
}
