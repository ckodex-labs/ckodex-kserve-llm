import { OperatorRouteState } from "@/components/layout/OperatorRouteState";

export default function Loading() {
    return (
        <OperatorRouteState
            description="Server sources and policy gates are being evaluated. The current route remains unqualified until resolution completes."
            kind="loading"
            title="Resolving operator route"
        />
    );
}
