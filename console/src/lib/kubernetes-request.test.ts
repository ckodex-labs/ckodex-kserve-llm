import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createServer } from "node:net";
import test from "node:test";

import * as k8s from "@kubernetes/client-node";

import {
    classifyKubernetesFailure,
    defaultKubernetesRequestTimeoutMs,
    kubernetesFailurePhrase,
    kubernetesRequestOptions,
    maximumKubernetesRequestTimeoutMs,
    minimumKubernetesRequestTimeoutMs,
    resolveKubernetesRequestTimeout,
} from "./kubernetes-request.ts";

test("bounds Kubernetes request deadlines from configuration", () => {
    assert.equal(resolveKubernetesRequestTimeout(undefined), defaultKubernetesRequestTimeoutMs);
    assert.equal(resolveKubernetesRequestTimeout("invalid"), defaultKubernetesRequestTimeoutMs);
    assert.equal(resolveKubernetesRequestTimeout("100"), minimumKubernetesRequestTimeoutMs);
    assert.equal(resolveKubernetesRequestTimeout("2500"), 2_500);
    assert.equal(resolveKubernetesRequestTimeout("90000"), maximumKubernetesRequestTimeoutMs);
});

test("attaches an abort signal through the typed Kubernetes request middleware", async () => {
    let signal: AbortSignal | undefined;
    const context = {
        setSignal(nextSignal: AbortSignal) {
            signal = nextSignal;
        },
    };
    const options = kubernetesRequestOptions(10);
    const middleware = options.middleware?.[0];

    assert.ok(middleware);
    await middleware.pre(context as never).toPromise();
    assert.equal(signal?.aborted, false);
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.equal(signal?.aborted, true);
    assert.equal(options.middlewareMergeStrategy, "append");
});

test("aborts a Kubernetes client request when the API socket stalls", { timeout: 2_000 }, async () => {
    const sockets = new Set<import("node:net").Socket>();
    const server = createServer((socket) => {
        sockets.add(socket);
        socket.once("close", () => sockets.delete(socket));
        // The accepted socket deliberately emits no HTTP response.
    });
    await new Promise<void>((resolve, reject) => {
        server.once("error", reject);
        server.listen(0, "127.0.0.1", resolve);
    });
    const address = server.address();
    assert.ok(address && typeof address === "object");
    const kc = new k8s.KubeConfig();
    kc.loadFromOptions({
        clusters: [{ name: "stalled", server: `http://127.0.0.1:${address.port}`, skipTLSVerify: true }],
        users: [{ name: "stalled" }],
        contexts: [{ name: "stalled", cluster: "stalled", user: "stalled" }],
        currentContext: "stalled",
    });
    const api = kc.makeApiClient(k8s.CoreV1Api);
    const startedAt = Date.now();
    let rejection: unknown;

    try {
        await api.listNamespace(undefined, kubernetesRequestOptions(50));
    } catch (error) {
        rejection = error;
    } finally {
        for (const socket of sockets) socket.destroy();
        await new Promise<void>((resolve, reject) => {
            server.close((error) => error ? reject(error) : resolve());
        });
    }

    assert.equal(classifyKubernetesFailure(rejection).kind, "timeout");
    assert.ok(Date.now() - startedAt < 750, "stalled request exceeded the bounded acceptance window");
});

test("separates deadlines and Kubernetes policy responses", () => {
    assert.deepEqual(classifyKubernetesFailure({ name: "AbortError" }), { kind: "timeout", code: null });
    assert.deepEqual(classifyKubernetesFailure({ code: 401 }), { kind: "unauthorized", code: "401" });
    assert.deepEqual(classifyKubernetesFailure({ code: 403 }), { kind: "forbidden", code: "403" });
    assert.deepEqual(classifyKubernetesFailure({ code: 404 }), { kind: "not-found", code: "404" });
    assert.deepEqual(classifyKubernetesFailure({ code: "ECONNREFUSED" }), { kind: "unavailable", code: "ECONNREFUSED" });
    assert.equal(kubernetesFailurePhrase({ code: "ETIMEDOUT" }, 750), "reached the 750 ms console deadline");
});

test("wires the bounded request policy into every Kubernetes source adapter", () => {
    const read = (file: string) => readFileSync(new URL(file, import.meta.url), "utf8");
    const kserve = read("./kserve.ts");
    const identity = read("./identity.server.ts");
    const workload = read("./workload.server.ts");

    assert.match(kserve, /listClusterCustomObject\([\s\S]*?kubernetesRequestOptions\(timeoutMs\)\)/);
    assert.match(kserve, /kubernetesResponseBody\(response\)/);
    assert.match(kserve, /listNamespace\(undefined, kubernetesRequestOptions\(timeoutMs\)\)/);
    assert.equal(kserve.match(/listNode\(undefined, kubernetesRequestOptions\(timeoutMs\)\)/g)?.length, 2);
    assert.match(identity, /listNamespacedConfigMap\([\s\S]*?kubernetesRequestOptions\(timeoutMs\)/);
    assert.match(identity, /createSelfSubjectReview\([\s\S]*?requestOptions\)/);
    assert.match(identity, /createSelfSubjectAccessReview\([\s\S]*?requestOptions\)/);
    assert.match(workload, /listNamespacedDeployment\([^\n]+requestOptions\)/);
    assert.match(workload, /listNamespacedPod\([^\n]+requestOptions\)/);
    assert.match(workload, /listNamespacedEvent\([\s\S]*?requestOptions\)/);
});
