export const investigationQueryLimit = 200;

export type InvestigationFilterKey = "outcome" | "readiness" | "state";

export interface InvestigationSearchState {
    filterKey: InvestigationFilterKey;
    filterValue: string;
    query: string;
}

export function boundedInvestigationQuery(value: string | string[] | undefined) {
    return typeof value === "string" ? value.slice(0, investigationQueryLimit) : "";
}

export function allowedInvestigationFilter<T extends string>(
    value: string | string[] | undefined,
    allowed: readonly T[],
    fallback: T,
) {
    return typeof value === "string" && allowed.includes(value as T) ? value as T : fallback;
}

export function buildInvestigationPath(currentHref: string, state: InvestigationSearchState) {
    const url = new URL(currentHref);
    const query = state.query.trim().slice(0, investigationQueryLimit);

    if (query) url.searchParams.set("q", query);
    else url.searchParams.delete("q");

    if (state.filterValue && state.filterValue !== "all") {
        url.searchParams.set(state.filterKey, state.filterValue);
    } else {
        url.searchParams.delete(state.filterKey);
    }

    return `${url.pathname}${url.search}${url.hash}`;
}
