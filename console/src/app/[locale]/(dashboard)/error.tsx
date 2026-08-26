"use client";

import { useEffect } from "react";
import { OperatorRouteState } from "@/components/layout/OperatorRouteState";

export default function ErrorBoundary({
    error,
    unstable_retry,
}: {
    error: Error & { digest?: string };
    unstable_retry: () => void;
}) {
    useEffect(() => {
        console.error("OPERATOR_ROUTE_RENDER_ERROR", error);
    }, [error]);

    return (
        <OperatorRouteState
            description="The route segment did not complete its render. The runtime reference can be correlated with server logs before a retry."
            kind="error"
            onRetry={unstable_retry}
            reference={error.digest}
            title="Route render interrupted"
        />
    );
}
