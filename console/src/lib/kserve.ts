import * as k8s from '@kubernetes/client-node';
import { resolveDeploymentProfile, type DeploymentProfile } from './config';
import { kubernetesFailurePhrase, kubernetesRequestOptions, kubernetesRequestTimeoutMs } from './kubernetes-request';
import { kubernetesResponseBody } from './kubernetes-response';
import type { SourceSnapshot } from './source';

export type { SourceSnapshot, SourceState } from './source';

/**
 * Resolves the deployment profile for the current environment.
 */
export async function getDeploymentProfile(): Promise<DeploymentProfile> {
    return resolveDeploymentProfile();
}

export interface InferenceService {
    name: string;
    namespace: string;
    uri: string;
    replicas: number;
    desiredReplicas: number | string;
    maxReplicas: number | string;
    ready: boolean;
    url: string;
    generation: number | null;
    observedGeneration: number | null;
    modelRevision: string;
    conditions: InferenceCondition[];
}

export interface InferenceCondition {
    type: string;
    status: string;
    reason: string;
    message: string;
    lastTransitionTime: string | null;
}

export interface InferenceInventorySource extends SourceSnapshot<InferenceService[]> {
    apiVersion: string;
    scope: string;
}

interface CustomResourceList<T> {
    items?: T[];
}

interface LLMInferenceResource {
    metadata?: { name?: string; namespace?: string; generation?: number };
    spec?: {
        model?: { uri?: string; name?: string };
        replicas?: number;
        scaling?: { maxReplicas?: number | string };
    };
    status?: {
        replicas?: number;
        modelReady?: boolean;
        url?: string;
        modelRevision?: string;
        observedGeneration?: number;
        conditions?: Array<{
            type?: string;
            status?: string;
            reason?: string;
            message?: string;
            lastTransitionTime?: string;
        }>;
    };
}

function safeHttpUrl(value: string | undefined): string {
    if (!value) return '';
    try {
        const url = new URL(value);
        return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : '';
    } catch {
        return '';
    }
}

export interface LatticeComponent {
    name: string;
    namespace: string;
    status: 'Present' | 'Missing';
    message: string;
}

export interface LatticeHealth {
    components: LatticeComponent[];
}

export type LatticeHealthSource = SourceSnapshot<LatticeHealth>;

export interface K8sNode {
    name: string;
    status: string;
    role: string;
    architecture: string;
    os: string;
    cpu: string;
    memory: string;
    age: string;
}

export type NodeSource = SourceSnapshot<K8sNode[]>;

export interface Accelerator {
    id: string;
    resourceName: string;
    type: 'GPU' | 'TPU' | 'AI accelerator';
    node: string;
    allocatable: string;
}

export type AcceleratorSource = SourceSnapshot<Accelerator[]>;

/**
 * Fetches LLMInferenceServices and preserves whether the source was observed,
 * empty, or unavailable.
 */
export async function getInferenceInventory(): Promise<InferenceInventorySource> {
    const apiVersion = 'serving.ckodex.com/v1';
    const scope = 'all namespaces';
    const timeoutMs = kubernetesRequestTimeoutMs();

    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const k8sApi = kc.makeApiClient(k8s.CustomObjectsApi);

        const response = await k8sApi.listClusterCustomObject({
            group: 'serving.ckodex.com',
            version: 'v1',
            plural: 'llminferenceservices'
        }, kubernetesRequestOptions(timeoutMs));

        const body = kubernetesResponseBody(response) as CustomResourceList<LLMInferenceResource>;
        const items = body?.items || [];

        const services = items.map((item) => {
            const uri = item.spec?.model?.uri || "Not reported";
            const status = item.status || {};
            return {
                name: item.metadata?.name || "unnamed",
                namespace: item.metadata?.namespace || "unknown",
                uri: uri,
                replicas: status.replicas ?? 0,
                desiredReplicas: item.spec?.replicas ?? "Not set",
                maxReplicas: item.spec?.scaling?.maxReplicas ?? "Not set",
                ready: status.modelReady ?? false,
                url: safeHttpUrl(status.url),
                generation: item.metadata?.generation ?? null,
                observedGeneration: status.observedGeneration ?? null,
                modelRevision: status.modelRevision || "Not reported",
                conditions: (status.conditions || []).map((condition) => ({
                    type: condition.type || "Unknown",
                    status: condition.status || "Unknown",
                    reason: condition.reason || "Not reported",
                    message: condition.message || "No message returned.",
                    lastTransitionTime: condition.lastTransitionTime || null,
                })),
            };
        });
        const observedAt = new Date().toISOString();

        return {
            state: services.length === 0 ? 'empty' : 'observed',
            data: services,
            source: `${apiVersion} · ${scope}`,
            observedAt,
            detail: services.length === 0
                ? 'The cluster-scoped request returned no LLMInferenceService resources.'
                : `${services.length} LLMInferenceService resource${services.length === 1 ? '' : 's'} returned.`,
            apiVersion,
            scope,
        };
    } catch (error: unknown) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`K8s API unavailable (Inferences): ${failure}.`);
        return {
            state: 'unavailable',
            data: [],
            source: `${apiVersion} · ${scope}`,
            observedAt: null,
            detail: `The Kubernetes inventory request ${failure}. Check endpoint reachability, the active context, served API version, and permissions.`,
            apiVersion,
            scope,
        };
    }
}

/**
 * Observes the presence of namespaces required by enabled control-plane features.
 */
export async function getLatticeHealthSource(): Promise<LatticeHealthSource> {
    const timeoutMs = kubernetesRequestTimeoutMs();
    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const k8sApi = kc.makeApiClient(k8s.CoreV1Api);

        const namespaces = await k8sApi.listNamespace(undefined, kubernetesRequestOptions(timeoutMs));
        const nsNames = namespaces.items.map((ns: k8s.V1Namespace) => ns.metadata?.name);
        const profile = resolveDeploymentProfile();

        const components: LatticeComponent[] = [
            {
                name: 'Core lattice namespace',
                namespace: 'lattice-system',
                status: nsNames.includes('lattice-system') ? 'Present' : 'Missing',
                message: nsNames.includes('lattice-system') ? 'Namespace returned by core/v1.' : 'Namespace not returned by core/v1.'
            }
        ];

        if (profile.features.istio) {
            components.push({
                name: 'Istio namespace',
                namespace: 'istio-system',
                status: nsNames.includes('istio-system') ? 'Present' : 'Missing',
                message: nsNames.includes('istio-system') ? 'Namespace returned by core/v1.' : 'Namespace not returned by core/v1.'
            });
        }

        if (profile.features.keda) {
            components.push({
                name: 'KEDA namespace',
                namespace: 'keda',
                status: nsNames.includes('keda') ? 'Present' : 'Missing',
                message: nsNames.includes('keda') ? 'Namespace returned by core/v1.' : 'Namespace not returned by core/v1.'
            });
        }

        return {
            state: 'observed',
            data: {
                components,
            },
            source: 'Kubernetes core/v1 namespaces',
            observedAt: new Date().toISOString(),
            detail: `${components.length} enabled control-plane check${components.length === 1 ? '' : 's'} evaluated.`,
        };
    } catch (error) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`K8s API unavailable (Namespace checks): ${failure}.`);
        return {
            state: 'unavailable',
            data: {
                components: [],
            },
            source: 'Kubernetes core/v1 namespaces',
            observedAt: null,
            detail: `The namespace-presence request ${failure}. Check endpoint reachability, the active context, and namespace-list permissions.`,
        };
    }
}

/**
 * Fetches all nodes in the Kubernetes cluster.
 */
export async function getNodeSource(): Promise<NodeSource> {
    const timeoutMs = kubernetesRequestTimeoutMs();
    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const k8sApi = kc.makeApiClient(k8s.CoreV1Api);

        const response = await k8sApi.listNode(undefined, kubernetesRequestOptions(timeoutMs));
        const nodes = response.items;

        const data = nodes.map((node) => ({
            name: node.metadata?.name || 'unnamed',
            status: node.status?.conditions?.find((condition) => condition.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady',
            role: Object.keys(node.metadata?.labels || {})
                .filter((label) => label.startsWith('node-role.kubernetes.io/'))
                .map((label) => label.slice('node-role.kubernetes.io/'.length))
                .filter(Boolean)
                .join(', ') || 'not labeled',
            architecture: node.metadata?.labels?.['kubernetes.io/arch'] || 'unknown',
            os: node.metadata?.labels?.['kubernetes.io/os'] || 'unknown',
            cpu: node.status?.capacity?.cpu || '0',
            memory: node.status?.capacity?.memory || '0',
            age: node.metadata?.creationTimestamp ?
                Math.floor((Date.now() - new Date(node.metadata.creationTimestamp).getTime()) / 86400000) + 'd' : 'unknown'
        }));

        return {
            state: data.length === 0 ? 'empty' : 'observed',
            data,
            source: 'Kubernetes core/v1 nodes',
            observedAt: new Date().toISOString(),
            detail: data.length === 0
                ? 'The active context returned no Node resources.'
                : `${data.length} Node resource${data.length === 1 ? '' : 's'} returned.`,
        };
    } catch (error: unknown) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`K8s API unavailable (Nodes): ${failure}.`);
        return {
            state: 'unavailable',
            data: [],
            source: 'Kubernetes core/v1 nodes',
            observedAt: null,
            detail: `The node inventory request ${failure}. Check endpoint reachability, the active context, and node-list permissions.`,
        };
    }
}

const acceleratorResources: Record<string, Accelerator['type']> = {
    'nvidia.com/gpu': 'GPU',
    'amd.com/gpu': 'GPU',
    'intel.com/gpu': 'GPU',
    'google.com/tpu': 'TPU',
    'habana.ai/gaudi': 'AI accelerator',
};

export async function getAcceleratorSource(): Promise<AcceleratorSource> {
    const timeoutMs = kubernetesRequestTimeoutMs();
    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const k8sApi = kc.makeApiClient(k8s.CoreV1Api);

        const response = await k8sApi.listNode(undefined, kubernetesRequestOptions(timeoutMs));
        const nodes = response.items;
        const accelerators: Accelerator[] = [];

        nodes.forEach((node) => {
            const allocatable = node.status?.allocatable || {};
            const nodeName = node.metadata?.name || 'unknown';

            for (const [resourceName, type] of Object.entries(acceleratorResources)) {
                const quantity = allocatable[resourceName];
                if (!quantity || Number.parseInt(quantity, 10) <= 0) continue;

                accelerators.push({
                    id: `${nodeName}:${resourceName}`,
                    resourceName,
                    type,
                    node: nodeName,
                    allocatable: quantity,
                });
            }
        });

        return {
            state: accelerators.length === 0 ? 'empty' : 'observed',
            data: accelerators,
            source: 'Kubernetes core/v1 nodes · status.allocatable',
            observedAt: new Date().toISOString(),
            detail: accelerators.length === 0
                ? 'No supported accelerator resource keys were reported as allocatable.'
                : `${accelerators.length} node-to-resource allocation${accelerators.length === 1 ? '' : 's'} returned. Utilization and device memory are not exposed by this source.`,
        };
    } catch (error: unknown) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`K8s API unavailable (Accelerators): ${failure}.`);
        return {
            state: 'unavailable',
            data: [],
            source: 'Kubernetes core/v1 nodes · status.allocatable',
            observedAt: null,
            detail: `The accelerator inventory request ${failure}. Check endpoint reachability, the active context, and node-list permissions.`,
        };
    }
}
