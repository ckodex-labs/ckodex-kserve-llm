export type SourceState = "observed" | "empty" | "partial" | "unavailable" | "malformed";

export interface SourceSnapshot<T> {
    state: SourceState;
    data: T;
    source: string;
    observedAt: string | null;
    detail: string;
}

export function isSourceObserved(source: Pick<SourceSnapshot<unknown>, "state">) {
    return source.state === "observed" || source.state === "empty";
}

export function sourceClaim(label: string, state: SourceState) {
    if (state === "observed") return `⊢ ${label} observed`;
    if (state === "empty") return `⊢ ${label} empty`;
    if (state === "partial" || state === "malformed") return `○ ${label} partial`;
    return `○ ${label} unavailable`;
}

export function formatObservationTime(value: string | null) {
    if (!value) return "No successful observation";

    return new Date(value).toLocaleString([], {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    });
}
