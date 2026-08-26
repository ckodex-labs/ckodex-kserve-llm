"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useTransition, type ReactNode } from "react";
import { useRouter } from "next/navigation";

interface OperatorRefreshState {
    isRefreshing: boolean;
    lastCompletedAt: Date | null;
    refreshSources: () => void;
}

const OperatorRefreshContext = createContext<OperatorRefreshState | null>(null);

export function OperatorRefreshProvider({ children }: { children: ReactNode }) {
    const router = useRouter();
    const [isPending, startTransition] = useTransition();
    const [lastCompletedAt, setLastCompletedAt] = useState<Date | null>(null);
    const refreshRequested = useRef(false);

    const refreshSources = useCallback(() => {
        if (refreshRequested.current) return;
        refreshRequested.current = true;
        startTransition(() => router.refresh());
    }, [router]);

    useEffect(() => {
        if (isPending || !refreshRequested.current) return;
        refreshRequested.current = false;
        let cancelled = false;
        queueMicrotask(() => {
            if (!cancelled) setLastCompletedAt(new Date());
        });
        return () => { cancelled = true; };
    }, [isPending]);

    const value = useMemo(() => ({
        isRefreshing: isPending,
        lastCompletedAt,
        refreshSources,
    }), [isPending, lastCompletedAt, refreshSources]);

    return <OperatorRefreshContext.Provider value={value}>{children}</OperatorRefreshContext.Provider>;
}

export function useOperatorRefresh() {
    const context = useContext(OperatorRefreshContext);
    if (!context) throw new Error("useOperatorRefresh must be used within OperatorRefreshProvider.");
    return context;
}
