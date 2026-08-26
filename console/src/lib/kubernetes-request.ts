import * as k8s from "@kubernetes/client-node";

export const defaultKubernetesRequestTimeoutMs = 5_000;
export const minimumKubernetesRequestTimeoutMs = 250;
export const maximumKubernetesRequestTimeoutMs = 30_000;

export type KubernetesFailureKind = "timeout" | "unauthorized" | "forbidden" | "not-found" | "unavailable";

export interface KubernetesFailure {
    kind: KubernetesFailureKind;
    code: string | null;
}

function errorField(error: unknown, field: "code" | "name") {
    if (typeof error !== "object" || error === null || !(field in error)) return null;
    const value = (error as Record<string, unknown>)[field];
    return typeof value === "string" || typeof value === "number" ? String(value) : null;
}

export function resolveKubernetesRequestTimeout(value: string | undefined) {
    if (!value || !/^\d+$/.test(value)) return defaultKubernetesRequestTimeoutMs;
    const parsed = Number(value);
    if (!Number.isSafeInteger(parsed)) return defaultKubernetesRequestTimeoutMs;
    return Math.min(maximumKubernetesRequestTimeoutMs, Math.max(minimumKubernetesRequestTimeoutMs, parsed));
}

export function kubernetesRequestTimeoutMs() {
    return resolveKubernetesRequestTimeout(process.env.CKODEX_KUBERNETES_REQUEST_TIMEOUT_MS);
}

export function kubernetesRequestOptions(timeoutMs = kubernetesRequestTimeoutMs()): k8s.ConfigurationOptions {
    const middleware: k8s.ObservableMiddleware = {
        pre(context) {
            context.setSignal(AbortSignal.timeout(timeoutMs));
            return new k8s.Observable(Promise.resolve(context));
        },
        post(context) {
            return new k8s.Observable(Promise.resolve(context));
        },
    };

    return {
        middleware: [middleware],
        middlewareMergeStrategy: "append",
    };
}

export function classifyKubernetesFailure(error: unknown): KubernetesFailure {
    const code = errorField(error, "code");
    const name = errorField(error, "name");
    const normalizedCode = code?.toUpperCase() || null;

    if (
        name === "AbortError" ||
        name === "TimeoutError" ||
        normalizedCode === "ABORT_ERR" ||
        normalizedCode === "ETIMEDOUT" ||
        normalizedCode === "UND_ERR_CONNECT_TIMEOUT"
    ) {
        return { kind: "timeout", code };
    }
    if (code === "401") return { kind: "unauthorized", code };
    if (code === "403") return { kind: "forbidden", code };
    if (code === "404") return { kind: "not-found", code };
    return { kind: "unavailable", code };
}

export function kubernetesFailurePhrase(error: unknown, timeoutMs = kubernetesRequestTimeoutMs()) {
    const failure = classifyKubernetesFailure(error);
    if (failure.kind === "timeout") return `reached the ${timeoutMs} ms console deadline`;
    if (failure.kind === "unauthorized") return "could not authenticate to Kubernetes (401)";
    if (failure.kind === "forbidden") return "was denied by Kubernetes (403)";
    if (failure.kind === "not-found") return "returned not found (404)";
    return failure.code ? `failed (${failure.code})` : "failed";
}
