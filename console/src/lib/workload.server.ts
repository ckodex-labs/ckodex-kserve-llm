import "server-only";

import * as k8s from "@kubernetes/client-node";

import { kubernetesFailurePhrase, kubernetesRequestOptions, kubernetesRequestTimeoutMs } from "./kubernetes-request";
import type { SourceSnapshot } from "./source";

export interface WorkloadDeployment {
    name: string;
    desired: number;
    ready: number;
    available: number;
    updated: number;
    generation: number | null;
    observedGeneration: number | null;
}

export interface WorkloadPod {
    name: string;
    phase: string;
    readyContainers: number;
    totalContainers: number;
    restarts: number;
    node: string;
    createdAt: string | null;
}

export interface WorkloadEvent {
    id: string;
    type: string;
    reason: string;
    message: string;
    count: number;
    reporter: string;
    firstAt: string | null;
    lastAt: string | null;
}

export interface WorkloadRuntime {
    deployments: WorkloadDeployment[];
    pods: WorkloadPod[];
    events: WorkloadEvent[];
}

export type WorkloadRuntimeSource = SourceSnapshot<WorkloadRuntime>;

const dnsLabel = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

function isNamespace(value: string) {
    return value.length > 0 && value.length <= 63 && dnsLabel.test(value);
}

function isResourceName(value: string) {
    if (value.length === 0 || value.length > 253) return false;
    return value.split(".").every((label) => label.length <= 63 && dnsLabel.test(label));
}

function isoTimestamp(value: unknown): string | null {
    if (value instanceof Date && !Number.isNaN(value.getTime())) return value.toISOString();
    if (typeof value !== "string" || Number.isNaN(Date.parse(value))) return null;
    return new Date(value).toISOString();
}

function latestEventTime(event: k8s.CoreV1Event) {
    return isoTimestamp(event.eventTime) || isoTimestamp(event.lastTimestamp) || isoTimestamp(event.firstTimestamp);
}

export async function getWorkloadRuntimeSource(namespace: string, name: string): Promise<WorkloadRuntimeSource> {
    const source = `Kubernetes apps/v1 + core/v1 · ns/${namespace}`;
    const emptyData: WorkloadRuntime = { deployments: [], pods: [], events: [] };
    const timeoutMs = kubernetesRequestTimeoutMs();
    const requestOptions = kubernetesRequestOptions(timeoutMs);

    if (!isNamespace(namespace) || !isResourceName(name)) {
        return {
            state: "unavailable",
            data: emptyData,
            source,
            observedAt: null,
            detail: "The requested namespace or workload name is not a valid Kubernetes identifier.",
        };
    }

    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const coreApi = kc.makeApiClient(k8s.CoreV1Api);
        const appsApi = kc.makeApiClient(k8s.AppsV1Api);
        const labelSelector = [
            "app.kubernetes.io/name=llminferenceservice",
            `app.kubernetes.io/instance=${name}`,
            "app.kubernetes.io/managed-by=ckodex-kserve-llm-operator",
        ].join(",");

        const results = await Promise.allSettled([
            appsApi.listNamespacedDeployment({ namespace, labelSelector }, requestOptions),
            coreApi.listNamespacedPod({ namespace, labelSelector }, requestOptions),
            coreApi.listNamespacedEvent({
                namespace,
                fieldSelector: `involvedObject.kind=LLMInferenceService,involvedObject.name=${name}`,
                limit: 100,
            }, requestOptions),
        ]);
        const [deploymentResult, podResult, eventResult] = results;

        const deployments = deploymentResult.status === "fulfilled"
            ? deploymentResult.value.items.map((deployment) => ({
                name: deployment.metadata?.name || "unnamed",
                desired: deployment.spec?.replicas ?? 0,
                ready: deployment.status?.readyReplicas ?? 0,
                available: deployment.status?.availableReplicas ?? 0,
                updated: deployment.status?.updatedReplicas ?? 0,
                generation: deployment.metadata?.generation ?? null,
                observedGeneration: deployment.status?.observedGeneration ?? null,
            }))
            : [];

        const pods = podResult.status === "fulfilled"
            ? podResult.value.items.map((pod) => {
                const containers = pod.status?.containerStatuses || [];
                return {
                    name: pod.metadata?.name || "unnamed",
                    phase: pod.status?.phase || "Unknown",
                    readyContainers: containers.filter((container) => container.ready).length,
                    totalContainers: containers.length,
                    restarts: containers.reduce((total, container) => total + container.restartCount, 0),
                    node: pod.spec?.nodeName || "Not scheduled",
                    createdAt: isoTimestamp(pod.metadata?.creationTimestamp),
                };
            })
            : [];

        const events = eventResult.status === "fulfilled"
            ? eventResult.value.items.map((event) => ({
                id: event.metadata?.uid || event.metadata?.name || `${event.reason || "event"}-${latestEventTime(event) || "unknown"}`,
                type: event.type || "Unknown",
                reason: event.reason || "Not reported",
                message: event.message || "No message returned.",
                count: event.count ?? event.series?.count ?? 1,
                reporter: event.reportingComponent || event.source?.component || "Not reported",
                firstAt: isoTimestamp(event.firstTimestamp),
                lastAt: latestEventTime(event),
            })).sort((left, right) => (right.lastAt || "").localeCompare(left.lastAt || ""))
            : [];

        const failures = results.filter((result): result is PromiseRejectedResult => result.status === "rejected");
        if (failures.length > 0) {
            console.warn(`Kubernetes workload context partial: ${failures.map((failure) => kubernetesFailurePhrase(failure.reason, timeoutMs)).join(", ")}.`);
        }

        const data = { deployments, pods, events };
        const returned = deployments.length + pods.length + events.length;
        const succeeded = results.length - failures.length;
        return {
            state: failures.length === results.length
                ? "unavailable"
                : failures.length > 0
                    ? "partial"
                    : returned === 0 ? "empty" : "observed",
            data,
            source,
            observedAt: succeeded > 0 ? new Date().toISOString() : null,
            detail: failures.length > 0
                ? `${succeeded} of ${results.length} workload-context requests succeeded. Returned ${deployments.length} deployment, ${pods.length} pod, and ${events.length} event records.`
                : `Returned ${deployments.length} deployment, ${pods.length} pod, and ${events.length} event records. Kubernetes Events are best-effort and retention-limited.`,
        };
    } catch (error) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        return {
            state: "unavailable",
            data: emptyData,
            source,
            observedAt: null,
            detail: `The Kubernetes workload-context source ${failure}. Check endpoint reachability, the active context, and permissions.`,
        };
    }
}
