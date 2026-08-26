"use client";

import { useEffect } from "react";

import {
    buildInvestigationPath,
    type InvestigationSearchState,
} from "@/lib/investigation-query";

export function useInvestigationSearchState(state: InvestigationSearchState) {
    const { filterKey, filterValue, query } = state;

    useEffect(() => {
        const nextPath = buildInvestigationPath(window.location.href, { filterKey, filterValue, query });
        const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
        if (nextPath !== currentPath) {
            window.history.replaceState(window.history.state, "", nextPath);
        }
    }, [filterKey, filterValue, query]);
}
