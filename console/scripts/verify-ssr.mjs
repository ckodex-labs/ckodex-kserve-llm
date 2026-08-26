import assert from "node:assert/strict";
import { spawn } from "node:child_process";

const port = Number(process.env.CKODEX_VERIFY_PORT || "3105");
if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
    throw new Error("CKODEX_VERIFY_PORT must be an unprivileged TCP port.");
}

const baseUrl = `http://127.0.0.1:${port}`;
const routes = [
    ["/en", "Reconciliation overview"],
    ["/en/fleet", "Inference fleet"],
    ["/en/events?q=default%2Fmodel-a", "Audit event ledger"],
    ["/en/identity", "Identity and authority"],
    ["/en/nodes", "Node inventory"],
    ["/en/accelerators", "Accelerator inventory"],
    ["/en/telemetry?q=model-a", "Runtime telemetry"],
    ["/en/terminal", "Command library"],
];

function count(html, tag) {
    return html.match(new RegExp(`<${tag}(?:\\s|>)`, "gi"))?.length || 0;
}

async function waitUntilReady() {
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
        try {
            const response = await fetch(`${baseUrl}/api/health`, { signal: AbortSignal.timeout(1_000) });
            if (response.ok) return;
        } catch {
            // Process startup is expected to reject connections briefly.
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
    }
    throw new Error("Console production server did not become ready within 15 seconds.");
}

const server = spawn(process.execPath, ["node_modules/next/dist/bin/next", "start", "-p", String(port)], {
    cwd: process.cwd(),
    env: { ...process.env, NODE_ENV: "production" },
    stdio: ["ignore", "pipe", "pipe"],
});

let serverOutput = "";
server.stdout.on("data", (chunk) => { serverOutput += chunk.toString(); });
server.stderr.on("data", (chunk) => { serverOutput += chunk.toString(); });

try {
    await waitUntilReady();

    const health = await fetch(`${baseUrl}/api/health`);
    assert.equal(health.status, 200);
    assert.equal(health.headers.get("cache-control"), "no-store");
    assert.equal(health.headers.get("x-powered-by"), null);
    assert.deepEqual(await health.json(), { status: "ok", scope: "console-process" });

    for (const [route, heading] of routes) {
        const response = await fetch(`${baseUrl}${route}`, { signal: AbortSignal.timeout(15_000) });
        assert.equal(response.status, 200, `${route} returned ${response.status}`);
        assert.equal(response.headers.get("x-powered-by"), null, `${route} exposes the framework header`);
        assert.ok(response.headers.get("content-security-policy"), `${route} has no CSP`);
        const html = await response.text();

        assert.match(html, /<html[^>]+data-theme="ledger"/, `${route} does not start in Ledger`);
        assert.ok(html.includes(heading), `${route} does not render ${heading}`);
        assert.equal(count(html, "main"), 1, `${route} must render one main landmark`);
        assert.equal(html.match(/<nav[^>]+aria-label="Primary navigation"/gi)?.length || 0, 1, `${route} must render one labeled primary navigation landmark`);
        assert.equal(count(html, "aside"), 1, `${route} must render one Evidence Margin landmark`);
        assert.equal(count(html, "footer"), 1, `${route} must render one authority footer`);
        assert.equal(count(html, "h1"), 1, `${route} must render one page heading`);

        const skip = html.indexOf('class="ck-skip"');
        const nav = html.indexOf("<nav");
        const main = html.indexOf("<main");
        assert.ok(skip >= 0 && nav >= 0 && main >= 0 && skip < nav && nav < main, `${route} has invalid skip/nav/main order`);
        assert.match(html, /<aside[^>]+ck-evidence-margin/, `${route} does not expose the Evidence Margin`);
        assert.match(html, /mode changes deployment, not governance semantics/, `${route} omits the authority invariant`);
    }

    const audit = await (await fetch(`${baseUrl}/en/events?q=default%2Fmodel-a`)).text();
    const telemetry = await (await fetch(`${baseUrl}/en/telemetry?q=model-a`)).text();
    assert.match(audit, /value="default\/model-a"/);
    assert.match(telemetry, /value="model-a"/);

    const missing = await fetch(`${baseUrl}/en/not-a-route`);
    assert.equal(missing.status, 404, "unmatched route must return 404");
    assert.equal(missing.headers.get("x-powered-by"), null, "unmatched route exposes the framework header");
    assert.ok(missing.headers.get("content-security-policy"), "unmatched route has no CSP");
    const missingHtml = await missing.text();
    assert.match(missingHtml, /<html[^>]+data-theme="ledger"/, "unmatched route does not start in Ledger");
    assert.match(missingHtml, /Operator surface unavailable/);
    assert.match(missingHtml, /No source or readiness observation is asserted/);
    assert.match(missingHtml, /mode changes deployment, not governance semantics/);
    assert.equal(count(missingHtml, "main"), 1, "unmatched route must render one main landmark");

    console.log(`SSR conformance passed for ${routes.length} console routes, global 404, and /api/health.`);
} catch (error) {
    if (serverOutput) process.stderr.write(serverOutput);
    throw error;
} finally {
    server.kill("SIGTERM");
}
