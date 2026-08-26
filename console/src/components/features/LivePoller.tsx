"use client";

import { useEffect, useState } from "react";

import { useOperatorRefresh } from "@/components/layout/OperatorRefreshProvider";

const refreshIntervalMs = 15_000;

/**
 * LivePoller component triggers a router refresh every 15 seconds.
 * This pattern allows React Server Components to re-fetch data
 * without manual useEffect state management in the main page.
 */
export function LivePoller() {
    const [active, setActive] = useState(true);
    const { isRefreshing, lastCompletedAt, refreshSources } = useOperatorRefresh();

    useEffect(() => {
        if (!active) return;

        const interval = setInterval(() => {
            if (document.visibilityState !== "visible") return;
            refreshSources();
        }, refreshIntervalMs);

        return () => clearInterval(interval);
    }, [active, refreshSources]);

    return (
        <div className="ml-auto flex items-center gap-3">
            <span aria-live="polite" className="hidden font-mono text-xs text-muted-foreground lg:inline">
                {isRefreshing
                    ? "Refreshing sources…"
                    : active
                        ? lastCompletedAt
                            ? `Refreshed ${lastCompletedAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`
                            : "Auto-refresh · 15s"
                        : "Auto-refresh paused"}
            </span>
            <button
                aria-pressed={!active}
                className="ck-refresh-control"
                disabled={isRefreshing}
                onClick={() => setActive((current) => !current)}
                type="button"
            >
                {active ? "Pause refresh" : "Resume refresh"}
            </button>
        </div>
    );
}
