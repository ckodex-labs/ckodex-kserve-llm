import type { SourceSnapshot } from "./source";

export type AuditOutcome = "Success" | "Failure" | "Denied";

export interface AuditEvent {
    action: string;
    resource: string;
    actor: string;
    outcome: AuditOutcome;
    timestamp: string;
    details: Record<string, string>;
    reason: string;
    execId: string;
    execKind: string;
    reproducibilityClass: string;
}

export type AuditLogSource = SourceSnapshot<AuditEvent[]>;

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function optionalString(value: unknown): string {
    return typeof value === "string" ? value : "";
}

function parseDetails(value: unknown): Record<string, string> | null {
    if (value === undefined) return {};
    if (!isRecord(value)) return null;

    const details: Record<string, string> = {};
    for (const [key, detail] of Object.entries(value)) {
        if (typeof detail !== "string") return null;
        details[key] = detail;
    }
    return details;
}

export function parseAuditEvent(line: string): AuditEvent | null {
    try {
        const value: unknown = JSON.parse(line);
        if (!isRecord(value)) return null;

        const { action, resource, actor, outcome, timestamp } = value;
        if (
            typeof action !== "string"
            || action.length === 0
            || typeof resource !== "string"
            || resource.length === 0
            || typeof actor !== "string"
            || actor.length === 0
            || (outcome !== "Success" && outcome !== "Failure" && outcome !== "Denied")
            || typeof timestamp !== "string"
            || Number.isNaN(Date.parse(timestamp))
        ) {
            return null;
        }

        const details = parseDetails(value.details);
        if (details === null) return null;

        return {
            action,
            resource,
            actor,
            outcome,
            timestamp,
            details,
            reason: optionalString(value.reason),
            execId: optionalString(value["exec.id"]),
            execKind: optionalString(value["exec.kind"]),
            reproducibilityClass: optionalString(value["exec.reproducibility_class"]),
        };
    } catch {
        return null;
    }
}

export function auditEventGlyph(outcome: AuditOutcome) {
    return outcome === "Success" ? "⊢" : "⊭";
}

export function auditEventSummary(event: AuditEvent) {
    if (event.reason) return event.reason;

    const firstDetail = Object.entries(event.details)[0];
    if (firstDetail) {
        const [key, value] = firstDetail;
        return `${key.replaceAll("_", " ")}: ${value}`;
    }

    return `${event.action} reported ${event.outcome.toLowerCase()}.`;
}
