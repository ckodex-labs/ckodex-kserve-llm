import assert from "node:assert/strict";
import test from "node:test";

import { assistantFailureDetail, createAssistantRequest } from "./assistant-request.ts";

function aborted(signal: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        if (signal.aborted) {
            resolve();
            return;
        }
        // AbortSignal.timeout uses an unref'd timer in some Node releases. Keep
        // this test alive with a bounded, referenced timer so the assertion
        // remains deterministic across supported CI runtimes.
        const guard = setTimeout(() => reject(new Error("abort signal did not fire")), 1_000);
        signal.addEventListener(
            "abort",
            () => {
                clearTimeout(guard);
                resolve();
            },
            { once: true },
        );
    });
}

test("cancels an assistant request with an explicit operator reason", async () => {
    const request = createAssistantRequest(1_000);
    const completion = aborted(request.signal);

    request.cancel("operator");
    await completion;

    assert.equal(request.signal.aborted, true);
    assert.equal(request.classifyFailure(request.signal.reason), "operator-cancelled");
    assert.equal(assistantFailureDetail("operator-cancelled", 1_000), "Response stopped by operator.");
});

test("distinguishes lifecycle cancellation from an elapsed deadline", async () => {
    const lifecycle = createAssistantRequest(1_000);
    lifecycle.cancel("lifecycle");
    assert.equal(lifecycle.classifyFailure(lifecycle.signal.reason), "lifecycle-cancelled");

    const deadline = createAssistantRequest(20);
    await aborted(deadline.signal);
    assert.equal(deadline.classifyFailure(deadline.signal.reason), "deadline");
    assert.equal(
        assistantFailureDetail("deadline", 35_000),
        "The assistant request reached the 35 s console deadline.",
    );
});

test("rejects invalid assistant deadlines", () => {
    assert.throws(() => createAssistantRequest(0), RangeError);
    assert.throws(() => createAssistantRequest(Number.NaN), RangeError);
});
