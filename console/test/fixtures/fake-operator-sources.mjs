import { createServer } from "node:http";

const port = Number(process.env.CKODEX_FAKE_SOURCES_PORT || "3121");
if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
    throw new Error("CKODEX_FAKE_SOURCES_PORT must be an unprivileged TCP port.");
}

const now = Date.now();
const isoMinutesAgo = (minutes) => new Date(now - minutes * 60_000).toISOString();
const unixMinutesAgo = (minutes) => Math.floor((now - minutes * 60_000) / 1000);

const inferenceServices = [
    {
        apiVersion: "serving.ckodex.com/v1",
        kind: "LLMInferenceService",
        metadata: { name: "orion-70b", namespace: "default", generation: 7 },
        spec: {
            model: { name: "orion-70b", uri: "oci://registry.internal/models/orion-70b@sha256:7f10a2" },
            replicas: 2,
            scaling: { maxReplicas: 4 },
        },
        status: {
            replicas: 2,
            modelReady: true,
            url: "https://orion-70b.default.example.internal",
            modelRevision: "sha256:7f10a2",
            observedGeneration: 7,
            conditions: [
                { type: "Ready", status: "True", reason: "RuntimeAvailable", message: "Runtime and model artifact are available.", lastTransitionTime: isoMinutesAgo(12) },
                { type: "PolicyReady", status: "True", reason: "PolicyAccepted", message: "Declared model source satisfied the configured policy gate.", lastTransitionTime: isoMinutesAgo(18) },
            ],
        },
    },
    {
        apiVersion: "serving.ckodex.com/v1",
        kind: "LLMInferenceService",
        metadata: { name: "summarizer-edge", namespace: "tenant-a", generation: 3 },
        spec: {
            model: { name: "summarizer-edge", uri: "hf://ckodex/summarizer-edge" },
            replicas: 1,
            scaling: { maxReplicas: 6 },
        },
        status: {
            replicas: 0,
            modelReady: false,
            modelRevision: "rev-2026-08-04",
            observedGeneration: 2,
            conditions: [
                { type: "Ready", status: "False", reason: "RevisionPending", message: "The latest declared generation has not been observed.", lastTransitionTime: isoMinutesAgo(4) },
            ],
        },
    },
];

const nodes = [
    {
        apiVersion: "v1",
        kind: "Node",
        metadata: {
            name: "gpu-a100-01",
            creationTimestamp: isoMinutesAgo(60 * 24 * 46),
            labels: { "kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux", "node-role.kubernetes.io/worker": "" },
        },
        status: {
            conditions: [{ type: "Ready", status: "True" }],
            capacity: { cpu: "64", memory: "512Gi" },
            allocatable: { cpu: "62", memory: "492Gi", "nvidia.com/gpu": "8" },
        },
    },
    {
        apiVersion: "v1",
        kind: "Node",
        metadata: {
            name: "gaudi2-02",
            creationTimestamp: isoMinutesAgo(60 * 24 * 19),
            labels: { "kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux", "node-role.kubernetes.io/worker": "" },
        },
        status: {
            conditions: [{ type: "Ready", status: "True" }],
            capacity: { cpu: "96", memory: "768Gi" },
            allocatable: { cpu: "94", memory: "742Gi", "habana.ai/gaudi": "8" },
        },
    },
    {
        apiVersion: "v1",
        kind: "Node",
        metadata: {
            name: "control-01",
            creationTimestamp: isoMinutesAgo(60 * 24 * 120),
            labels: { "kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux", "node-role.kubernetes.io/control-plane": "" },
        },
        status: {
            conditions: [{ type: "Ready", status: "False" }],
            capacity: { cpu: "16", memory: "64Gi" },
            allocatable: { cpu: "14", memory: "58Gi" },
        },
    },
];

const deployments = [
    {
        apiVersion: "apps/v1",
        kind: "Deployment",
        metadata: { name: "orion-70b-predictor", namespace: "default", generation: 7 },
        spec: { replicas: 2 },
        status: { readyReplicas: 2, availableReplicas: 2, updatedReplicas: 2, observedGeneration: 7 },
    },
];

const pods = ["orion-70b-predictor-7f8d9-a1", "orion-70b-predictor-7f8d9-b2"].map((name, index) => ({
    apiVersion: "v1",
    kind: "Pod",
    metadata: { name, namespace: "default", creationTimestamp: isoMinutesAgo(34 - index * 3) },
    spec: { nodeName: index === 0 ? "gpu-a100-01" : "gaudi2-02" },
    status: {
        phase: "Running",
        containerStatuses: [
            { name: "runtime", ready: true, restartCount: index },
            { name: "evidence-sidecar", ready: true, restartCount: 0 },
        ],
    },
}));

const events = [
    {
        apiVersion: "v1",
        kind: "Event",
        metadata: { name: "orion-70b-ready", namespace: "default", uid: "event-orion-ready" },
        type: "Normal",
        reason: "RuntimeReady",
        message: "Deployment reached the declared replica envelope.",
        count: 1,
        reportingComponent: "ckodex-kserve-llm-operator",
        firstTimestamp: isoMinutesAgo(18),
        lastTimestamp: isoMinutesAgo(12),
    },
    {
        apiVersion: "v1",
        kind: "Event",
        metadata: { name: "orion-70b-policy", namespace: "default", uid: "event-orion-policy" },
        type: "Normal",
        reason: "PolicyAccepted",
        message: "Model source and runtime policy checks completed.",
        count: 1,
        reportingComponent: "ckodex-kserve-llm-operator",
        firstTimestamp: isoMinutesAgo(20),
        lastTimestamp: isoMinutesAgo(18),
    },
];

const registrations = inferenceServices.map((service, index) => {
    const spiffeId = `spiffe://ckodex.test/ns/${service.metadata.namespace}/sa/${service.metadata.name}/model/${service.metadata.name}`;
    return {
        apiVersion: "v1",
        kind: "ConfigMap",
        metadata: {
            name: `spire-entry-${service.metadata.name}`,
            namespace: "spire",
            uid: `spire-entry-${index + 1}`,
            labels: { "spire.ckodex.com/registration-entry": "true", "ckodex.com/tenant-id": index === 0 ? "platform" : "tenant-a" },
            annotations: {
                "ckodex.com/spiffe-id": spiffeId,
                "ckodex.com/source-namespace": service.metadata.namespace,
                "ckodex.com/source-service": service.metadata.name,
            },
        },
        data: {
            "entry.json": JSON.stringify({
                spiffeId,
                parentId: "spiffe://ckodex.test/spire/agent/k8s_psat/cluster",
                selectors: [`k8s:ns:${service.metadata.namespace}`, `k8s:sa:${service.metadata.name}`],
                ttl: 3600,
                dnsSans: [`${service.metadata.name}.${service.metadata.namespace}.svc`],
            }),
        },
    };
});

const metricSeries = [
    ["ckodex:reconcile_success_rate", { model: "orion-70b", namespace: "default" }, [0.94, 0.96, 0.97, 0.99]],
    ["ckodex:inference_latency_p99", { model: "orion-70b", namespace: "default" }, [1.62, 1.48, 1.31, 1.27]],
    ["ckodex:gpu_utilization_p95", { node: "gpu-a100-01" }, [0.58, 0.71, 0.76, 0.68]],
    ["ckodex:tokens_per_tenant_hour", { tenant: "tenant-a" }, [180200, 194400, 210800, 223900]],
].map(([metric, labels, values]) => ({
    metric: { __name__: metric, ...labels },
    values: values.map((value, index) => [unixMinutesAgo((values.length - index) * 15), String(value)]),
}));

const alerts = [
    {
        activeAt: isoMinutesAgo(11),
        annotations: { summary: "The latest workload generation has not reconciled.", runbook_url: "https://docs.ckodex.com/runbooks/reconcile-errors" },
        labels: { alertname: "LLMGenerationDrift", model: "summarizer-edge", namespace: "tenant-a", team: "platform" },
        state: "firing",
        value: "1",
    },
    {
        activeAt: isoMinutesAgo(6),
        annotations: { summary: "Inference p99 latency is approaching the declared threshold." },
        labels: { alertname: "LLMLatencyBudget", model: "orion-70b", namespace: "default", team: "serving" },
        state: "pending",
        value: "1.27",
    },
];

function json(response, status, body) {
    response.writeHead(status, { "Cache-Control": "no-store", "Content-Type": "application/json; charset=utf-8" });
    response.end(JSON.stringify(body));
}

async function requestBody(request) {
    let body = "";
    for await (const chunk of request) {
        body += chunk;
        if (body.length > 1_000_000) throw new Error("Fixture request body exceeded one megabyte.");
    }
    return body ? JSON.parse(body) : {};
}

const server = createServer(async (request, response) => {
    try {
        const url = new URL(request.url || "/", `http://127.0.0.1:${port}`);

        if (request.method === "GET" && url.pathname === "/apis/serving.ckodex.com/v1/llminferenceservices") {
            return json(response, 200, { apiVersion: "serving.ckodex.com/v1", kind: "LLMInferenceServiceList", metadata: {}, items: inferenceServices });
        }
        if (request.method === "GET" && url.pathname === "/api/v1/namespaces") {
            const items = ["default", "tenant-a", "lattice-system", "spire"].map((name) => ({ apiVersion: "v1", kind: "Namespace", metadata: { name } }));
            return json(response, 200, { apiVersion: "v1", kind: "NamespaceList", metadata: {}, items });
        }
        if (request.method === "GET" && url.pathname === "/api/v1/nodes") {
            return json(response, 200, { apiVersion: "v1", kind: "NodeList", metadata: {}, items: nodes });
        }
        if (request.method === "GET" && url.pathname === "/api/v1/namespaces/spire/configmaps") {
            return json(response, 200, { apiVersion: "v1", kind: "ConfigMapList", metadata: {}, items: registrations });
        }
        if (request.method === "GET" && url.pathname === "/apis/apps/v1/namespaces/default/deployments") {
            return json(response, 200, { apiVersion: "apps/v1", kind: "DeploymentList", metadata: {}, items: deployments });
        }
        if (request.method === "GET" && url.pathname === "/api/v1/namespaces/default/pods") {
            return json(response, 200, { apiVersion: "v1", kind: "PodList", metadata: {}, items: pods });
        }
        if (request.method === "GET" && url.pathname === "/api/v1/namespaces/default/events") {
            return json(response, 200, { apiVersion: "v1", kind: "EventList", metadata: {}, items: events });
        }
        if (request.method === "POST" && url.pathname === "/apis/authentication.k8s.io/v1/selfsubjectreviews") {
            await requestBody(request);
            return json(response, 201, {
                apiVersion: "authentication.k8s.io/v1",
                kind: "SelfSubjectReview",
                status: { userInfo: { username: "system:serviceaccount:ckodex-system:console", uid: "fixture-console-principal", groups: ["system:serviceaccounts", "system:authenticated"] } },
            });
        }
        if (request.method === "POST" && url.pathname === "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews") {
            const body = await requestBody(request);
            const verb = body?.spec?.resourceAttributes?.verb;
            const mutate = verb === "patch" || verb === "delete" || verb === "create" || verb === "update";
            return json(response, 201, {
                apiVersion: "authorization.k8s.io/v1",
                kind: "SelfSubjectAccessReview",
                status: mutate
                    ? { allowed: false, denied: true, reason: "Observe-only console policy." }
                    : { allowed: true, denied: false, reason: "Fixture read policy." },
            });
        }
        if (request.method === "GET" && url.pathname === "/prometheus/api/v1/query_range") {
            return json(response, 200, { status: "success", data: { resultType: "matrix", result: metricSeries } });
        }
        if (request.method === "GET" && url.pathname === "/prometheus/api/v1/alerts") {
            return json(response, 200, { status: "success", data: { alerts } });
        }

        return json(response, 404, { apiVersion: "v1", kind: "Status", status: "Failure", reason: "NotFound", code: 404, message: `${request.method} ${url.pathname} is not defined by the populated fixture.` });
    } catch (error) {
        return json(response, 500, { apiVersion: "v1", kind: "Status", status: "Failure", reason: "FixtureError", code: 500, message: error instanceof Error ? error.message : "Fixture request failed." });
    }
});

server.listen(port, "127.0.0.1", () => {
    console.log(`Populated operator sources listening on http://127.0.0.1:${port}`);
});
