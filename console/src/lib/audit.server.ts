import "server-only";

import { parseAuditEvent, type AuditEvent, type AuditLogSource } from "./audit";

const defaultAuditLogPath = "/var/log/ckodex/audit.jsonl";

export type { AuditEvent, AuditLogSource } from "./audit";

export async function getAuditLogSource(limit: number = 20): Promise<AuditLogSource> {
    const auditPath = process.env.CKODEX_AUDIT_LOG_PATH || defaultAuditLogPath;

    try {
        const { readFile } = await import("node:fs/promises");
        const data = await readFile(/* turbopackIgnore: true */ auditPath, "utf8");
        const trimmed = data.trim();
        const observedAt = new Date().toISOString();

        if (!trimmed) {
            return {
                state: "empty",
                data: [],
                source: auditPath,
                observedAt,
                detail: "The configured audit file exists and contains no records.",
            };
        }

        const lines = trimmed.split("\n").slice(-limit);
        const events = lines.map(parseAuditEvent).filter((event): event is AuditEvent => event !== null).reverse();
        const malformedCount = lines.length - events.length;

        return {
            state: malformedCount > 0 ? "malformed" : "observed",
            data: events,
            source: auditPath,
            observedAt,
            detail: malformedCount > 0
                ? `${malformedCount} audit record${malformedCount === 1 ? '' : 's'} could not be parsed; ${events.length} valid record${events.length === 1 ? '' : 's'} retained.`
                : `${events.length} audit record${events.length === 1 ? '' : 's'} returned.`,
        };
    } catch (error: unknown) {
        const code = typeof error === "object" && error !== null && "code" in error
            ? (error as { code?: unknown }).code
            : undefined;

        if (code === "ENOENT") {
            return {
                state: "unavailable",
                data: [],
                source: auditPath,
                observedAt: null,
                detail: "The configured audit file does not exist.",
            };
        }

        console.error("Audit Read Error:", error);
        return {
            state: "unavailable",
            data: [],
            source: auditPath,
            observedAt: null,
            detail: "The configured audit file could not be read. Check the path and file permissions.",
        };
    }
}

export async function getAuditLogs(limit: number = 20): Promise<AuditEvent[]> {
    return (await getAuditLogSource(limit)).data;
}
