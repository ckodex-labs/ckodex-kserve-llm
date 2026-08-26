"use client";

import { useOperatorRefresh } from "./OperatorRefreshProvider";

export function RefreshControl() {
    const { isRefreshing, refreshSources } = useOperatorRefresh();

    return (
        <button
            aria-busy={isRefreshing}
            className="ck-refresh-control"
            disabled={isRefreshing}
            onClick={refreshSources}
            type="button"
        >
            {isRefreshing ? "Refreshing sources…" : "Refresh sources"}
        </button>
    );
}
