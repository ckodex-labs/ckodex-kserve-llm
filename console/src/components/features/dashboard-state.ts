import type { AuditLogSource } from "@/lib/audit.server";
import type { InferenceInventorySource, LatticeHealthSource } from "@/lib/kserve";

export interface DashboardAttentionItem {
    id: string;
    label: string;
    source: string;
    detail: string;
}

export function isInventoryQualified(inventory: InferenceInventorySource) {
    return inventory.state === "observed" || inventory.state === "empty";
}

export function getDashboardAttention(
    inventory: InferenceInventorySource,
    health: LatticeHealthSource,
    audit: AuditLogSource,
): DashboardAttentionItem[] {
    const items: DashboardAttentionItem[] = [];

    if (!isInventoryQualified(inventory)) {
        items.push({
            id: "inventory-source",
            label: "Inference inventory unavailable",
            source: inventory.source,
            detail: inventory.detail,
        });
    } else {
        for (const service of inventory.data) {
            const generationCurrent = service.generation !== null
                && service.observedGeneration !== null
                && service.generation === service.observedGeneration;
            if (!service.ready || !generationCurrent) {
                items.push({
                    id: `inference-${service.name}`,
                    label: service.name,
                    source: inventory.source,
                    detail: `${service.ready ? "Ready reported" : "Not ready reported"}; generation ${service.observedGeneration ?? "not reported"} observed of ${service.generation ?? "unknown"} declared.`,
                });
            }
        }
    }

    if (health.state !== "observed") {
        items.push({
            id: "health-source",
            label: "Namespace checks unavailable",
            source: health.source,
            detail: health.detail,
        });
    } else {
        for (const component of health.data.components) {
            if (component.status !== "Present") {
                items.push({
                    id: `health-${component.namespace}-${component.name}`,
                    label: component.name,
                    source: `${health.source} · ${component.namespace}`,
                    detail: `${component.status}. ${component.message}`,
                });
            }
        }
    }

    if (audit.state === "unavailable" || audit.state === "malformed") {
        items.push({
            id: "audit-source",
            label: audit.state === "malformed" ? "Audit source is partial" : "Audit source unavailable",
            source: audit.source,
            detail: audit.detail,
        });
    }

    return items;
}
