export const spireRegistrationLabel = "spire.ckodex.com/registration-entry=true";

export interface SpireRegistrationConfigMap {
    metadata?: {
        name?: string;
        namespace?: string;
        uid?: string;
        labels?: Record<string, string>;
        annotations?: Record<string, string>;
    };
    data?: Record<string, string>;
}

export interface SpireRegistration {
    id: string;
    configMapName: string;
    registrationNamespace: string;
    spiffeId: string;
    trustDomain: string;
    parentId: string | null;
    selectors: string[];
    ttlSeconds: number | null;
    dnsSans: string[];
    sourceNamespace: string;
    sourceService: string;
    tenantId: string | null;
}

export type CapabilityDecision = "allowed" | "denied" | "no-opinion" | "unavailable";

export interface CapabilitySpec {
    id: string;
    label: string;
    effect: "observe" | "mutate";
    verb: string;
    group: string;
    version: string;
    resource: string;
    namespace: string | null;
}

export interface CapabilityResult extends CapabilitySpec {
    decision: CapabilityDecision;
    reason: string;
    evaluationError: string | null;
}

export interface PrincipalIdentity {
    username: string;
    uid: string | null;
    groups: string[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function boundedStrings(value: unknown, maximum: number): string[] | null {
    if (!Array.isArray(value) || value.length > maximum) return null;
    const strings = value.filter((item): item is string => typeof item === "string" && item.length > 0 && item.length <= 512);
    return strings.length === value.length ? strings : null;
}

export function parseSpiffeId(value: unknown): { id: string; trustDomain: string } | null {
    if (typeof value !== "string" || value.length === 0 || value.length > 2048) return null;

    try {
        const url = new URL(value);
        if (
            url.protocol !== "spiffe:"
            || !url.hostname
            || url.username
            || url.password
            || url.search
            || url.hash
            || url.pathname === "/"
        ) return null;

        return { id: url.toString(), trustDomain: url.hostname };
    } catch {
        return null;
    }
}

export function parseSpireRegistration(configMap: SpireRegistrationConfigMap): SpireRegistration | null {
    const name = configMap.metadata?.name;
    const namespace = configMap.metadata?.namespace;
    const entryJson = configMap.data?.["entry.json"];
    if (!name || !namespace || !entryJson || entryJson.length > 65_536) return null;

    let parsed: unknown;
    try {
        parsed = JSON.parse(entryJson);
    } catch {
        return null;
    }
    if (!isRecord(parsed)) return null;

    const identity = parseSpiffeId(parsed.spiffeId);
    const selectors = boundedStrings(parsed.selectors, 32);
    const dnsSans = parsed.dnsSans === undefined ? [] : boundedStrings(parsed.dnsSans, 64);
    const parent = parsed.parentId === undefined ? null : parseSpiffeId(parsed.parentId);
    const ttl = parsed.ttl;
    const ttlSeconds = ttl === undefined
        ? null
        : typeof ttl === "number" && Number.isSafeInteger(ttl) && ttl > 0 && ttl <= 2_147_483_647
            ? ttl
            : null;

    if (!identity || !selectors || !dnsSans || (parsed.parentId !== undefined && !parent)) return null;
    if (ttl !== undefined && ttlSeconds === null) return null;

    const annotations = configMap.metadata?.annotations || {};
    const annotationIdentity = annotations["ckodex.com/spiffe-id"];
    if (annotationIdentity && annotationIdentity !== identity.id) return null;

    return {
        id: configMap.metadata?.uid || `${namespace}/${name}`,
        configMapName: name,
        registrationNamespace: namespace,
        spiffeId: identity.id,
        trustDomain: identity.trustDomain,
        parentId: parent?.id || null,
        selectors,
        ttlSeconds,
        dnsSans,
        sourceNamespace: annotations["ckodex.com/source-namespace"] || "Not reported",
        sourceService: annotations["ckodex.com/source-service"] || "Not reported",
        tenantId: configMap.metadata?.labels?.["ckodex.com/tenant-id"] || null,
    };
}

export const consoleCapabilitySpecs: readonly CapabilitySpec[] = [
    { id: "inference-list", label: "List inference services", effect: "observe", verb: "list", group: "serving.ckodex.com", version: "v1", resource: "llminferenceservices", namespace: null },
    { id: "node-list", label: "List nodes", effect: "observe", verb: "list", group: "", version: "v1", resource: "nodes", namespace: null },
    { id: "namespace-list", label: "List namespaces", effect: "observe", verb: "list", group: "", version: "v1", resource: "namespaces", namespace: null },
    { id: "deployment-list", label: "List deployments", effect: "observe", verb: "list", group: "apps", version: "v1", resource: "deployments", namespace: null },
    { id: "pod-list", label: "List pods", effect: "observe", verb: "list", group: "", version: "v1", resource: "pods", namespace: null },
    { id: "event-list", label: "List events", effect: "observe", verb: "list", group: "", version: "v1", resource: "events", namespace: null },
    { id: "spire-registration-list", label: "List SPIRE registrations", effect: "observe", verb: "list", group: "", version: "v1", resource: "configmaps", namespace: "spire" },
    { id: "inference-patch", label: "Patch inference services", effect: "mutate", verb: "patch", group: "serving.ckodex.com", version: "v1", resource: "llminferenceservices", namespace: null },
    { id: "inference-delete", label: "Delete inference services", effect: "mutate", verb: "delete", group: "serving.ckodex.com", version: "v1", resource: "llminferenceservices", namespace: null },
];

export function capabilityDecision(allowed: boolean, denied: boolean | undefined): CapabilityDecision {
    if (allowed) return "allowed";
    return denied ? "denied" : "no-opinion";
}
