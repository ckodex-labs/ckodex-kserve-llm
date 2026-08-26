"use client";

import { useState } from "react";

const commands = [
    {
        id: "inference-inventory",
        label: "Inference inventory",
        command: "kubectl get llminferenceservices.serving.ckodex.com -A",
        detail: "List declared LLM inference services across namespaces.",
    },
    {
        id: "node-readiness",
        label: "Node readiness",
        command: "kubectl get nodes -o wide",
        detail: "Inspect node readiness and topology in the active context.",
    },
    {
        id: "recent-events",
        label: "Recent events",
        command: "kubectl get events -A --sort-by=.lastTimestamp",
        detail: "Review cluster events in chronological order.",
    },
    {
        id: "ci-fast-gate",
        label: "Repository fast gate",
        command: "dagger call all --source=.",
        detail: "Run the repository's hosted-equivalent lint and build check.",
    },
] as const;

export function FabricTerminalBlock() {
    const [copyResult, setCopyResult] = useState<{ id: string; state: "copied" | "failed" } | null>(null);

    async function copyCommand(id: string, command: string) {
        try {
            await navigator.clipboard.writeText(command);
            setCopyResult({ id, state: "copied" });
        } catch {
            setCopyResult({ id, state: "failed" });
        }
        window.setTimeout(() => setCopyResult((current) => current?.id === id ? null : current), 1600);
    }

    return (
        <section aria-labelledby="command-catalog-title">
            <div className="border border-border p-5">
                <p className="ck-label">Non-executing surface</p>
                <h2 className="mt-2 text-xl font-semibold" id="command-catalog-title">Operator command catalog</h2>
                <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                    Copy reviewed, read-only commands to a trusted local shell. This console does not currently open a cluster shell or execute commands.
                </p>
            </div>

            <div className="mt-px border border-border">
                {commands.map((item) => (
                    <article className="grid gap-4 border-b border-border p-5 last:border-b-0 lg:grid-cols-[minmax(180px,0.7fr)_minmax(300px,1.5fr)_auto] lg:items-center" key={item.id}>
                        <div>
                            <h3 className="text-sm font-semibold">{item.label}</h3>
                            <p className="mt-1 text-xs leading-5 text-muted-foreground">{item.detail}</p>
                        </div>
                        <code className="overflow-x-auto border border-border bg-muted p-3 font-mono text-xs leading-5">{item.command}</code>
                        <button
                            className="ck-refresh-control min-w-[112px]"
                            onClick={() => copyCommand(item.id, item.command)}
                            type="button"
                        >
                            {copyResult?.id === item.id
                                ? copyResult.state === "copied" ? "Copied" : "Copy failed"
                                : "Copy command"}
                        </button>
                    </article>
                ))}
            </div>
            <p className="mt-4 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground" role="status" aria-live="polite">
                {copyResult?.state === "copied"
                    ? "⊢ Command copied to clipboard"
                    : copyResult?.state === "failed"
                        ? "○ Clipboard access unavailable"
                        : "○ No command executed by this surface"}
            </p>
        </section>
    );
}
