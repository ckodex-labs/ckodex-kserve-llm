import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const consolePort = checkedPort(process.env.CKODEX_POPULATED_VERIFY_PORT || "3106", "CKODEX_POPULATED_VERIFY_PORT");
const sourcePort = checkedPort(process.env.CKODEX_POPULATED_SOURCE_PORT || "3122", "CKODEX_POPULATED_SOURCE_PORT");
const baseUrl = `http://127.0.0.1:${consolePort}`;
const sourceUrl = `http://127.0.0.1:${sourcePort}`;
const temporaryDirectory = await mkdtemp(join(tmpdir(), "ckodex-populated-"));
const kubeconfigPath = join(temporaryDirectory, "kubeconfig.yaml");

const routes = [
    ["/en", ["Reconciliation overview", "orion-70b", "inventory observed", "namespace checks observed", "audit observed"]],
    ["/en/fleet", ["Inference fleet", "orion-70b", "summarizer-edge", "inventory observed"]],
    ["/en/fleet/default/orion-70b", ["orion-70b-predictor", "orion-70b-predictor-7f8d9-a1", "RuntimeReady"]],
    ["/en/events", ["Audit event ledger", "PolicyViolation", "LLMInferenceService/default/model-a", "audit observed"]],
    ["/en/identity", ["Identity and authority", "system:serviceaccount:ckodex-system:console", "spiffe://ckodex.test/ns/default", "principal observed"]],
    ["/en/nodes", ["Node inventory", "gpu-a100-01", "gaudi2-02", "nodes observed"]],
    ["/en/accelerators", ["Accelerator inventory", "nvidia.com/gpu", "habana.ai/gaudi", "capacity observed"]],
    ["/en/telemetry", ["Runtime telemetry", "LLMGenerationDrift", "223,900", "telemetry observed"]],
    ["/en/terminal", ["Command library", "dagger call all --source=.", "No command executed", "execution unavailable"]],
];

function checkedPort(value, name) {
    const port = Number(value);
    if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
        throw new Error(`${name} must be an unprivileged TCP port.`);
    }
    return port;
}

function processOutput(child) {
    let output = "";
    child.stdout.on("data", (chunk) => { output += chunk.toString(); });
    child.stderr.on("data", (chunk) => { output += chunk.toString(); });
    return () => output;
}

async function waitUntilReady(url, label) {
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
        try {
            const response = await fetch(url, { signal: AbortSignal.timeout(1_000) });
            if (response.ok) return;
        } catch {
            // Local processes reject connections briefly during startup.
        }
        await new Promise((resolveReady) => setTimeout(resolveReady, 100));
    }
    throw new Error(`${label} did not become ready within 15 seconds.`);
}

function assertLandmarks(html, route) {
    for (const tag of ["main", "aside", "footer", "h1"]) {
        const count = html.match(new RegExp(`<${tag}(?:\\s|>)`, "gi"))?.length || 0;
        assert.equal(count, 1, `${route} must render one ${tag} landmark`);
    }
    const primaryNavigation = html.match(/<nav[^>]+aria-label="Primary navigation"/gi)?.length || 0;
    assert.equal(primaryNavigation, 1, `${route} must render one labeled primary navigation landmark`);
    assert.match(html, /<html[^>]+data-theme="ledger"/, `${route} does not start in Ledger`);
    assert.match(html, /<aside[^>]+ck-evidence-margin/, `${route} does not expose the Evidence Margin`);
    assert.match(html, /mode changes deployment, not governance semantics/, `${route} omits the authority invariant`);
}

await writeFile(kubeconfigPath, `apiVersion: v1
kind: Config
clusters:
  - name: populated-console-fixture
    cluster:
      server: ${sourceUrl}
      insecure-skip-tls-verify: true
contexts:
  - name: populated-console-fixture
    context:
      cluster: populated-console-fixture
      user: populated-console-fixture
current-context: populated-console-fixture
users:
  - name: populated-console-fixture
    user: {}
`);

const source = spawn(process.execPath, ["test/fixtures/fake-operator-sources.mjs"], {
    cwd: process.cwd(),
    env: { ...process.env, CKODEX_FAKE_SOURCES_PORT: String(sourcePort) },
    stdio: ["ignore", "pipe", "pipe"],
});
const readSourceOutput = processOutput(source);
let consoleServer;
let readConsoleOutput = () => "";

try {
    await waitUntilReady(`${sourceUrl}/api/v1/nodes`, "Populated source fixture");

    consoleServer = spawn(process.execPath, ["node_modules/next/dist/bin/next", "start", "-p", String(consolePort)], {
        cwd: process.cwd(),
        env: {
            ...process.env,
            NODE_ENV: "production",
            KUBECONFIG: kubeconfigPath,
            CKODEX_AUDIT_LOG_PATH: resolve("test/fixtures/audit-runtime.jsonl"),
            CKODEX_PROMETHEUS_ALLOW_INSECURE: "true",
            CKODEX_PROMETHEUS_URL: `${sourceUrl}/prometheus`,
        },
        stdio: ["ignore", "pipe", "pipe"],
    });
    readConsoleOutput = processOutput(consoleServer);
    await waitUntilReady(`${baseUrl}/api/health`, "Populated console");

    for (const [route, expectedText] of routes) {
        const response = await fetch(`${baseUrl}${route}`, { signal: AbortSignal.timeout(15_000) });
        assert.equal(response.status, 200, `${route} returned ${response.status}`);
        assert.equal(response.headers.get("x-powered-by"), null, `${route} exposes the framework header`);
        assert.ok(response.headers.get("content-security-policy"), `${route} has no CSP`);
        const html = await response.text();
        assertLandmarks(html, route);
        for (const expected of expectedText) {
            assert.ok(html.includes(expected), `${route} does not render ${expected}`);
        }
    }

    console.log(`Populated-state verification passed for ${routes.length} operator routes.`);
} catch (error) {
    const output = `${readSourceOutput()}${readConsoleOutput()}`;
    if (output) process.stderr.write(output);
    throw error;
} finally {
    consoleServer?.kill("SIGTERM");
    source.kill("SIGTERM");
    await rm(temporaryDirectory, { force: true, recursive: true });
}
