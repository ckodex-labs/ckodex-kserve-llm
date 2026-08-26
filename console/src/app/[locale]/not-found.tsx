import { OperatorRouteState } from "@/components/layout/OperatorRouteState";

export default function NotFound() {
    return (
        <OperatorRouteState
            description="The requested surface is absent or excluded by the active deployment profile. Available operator routes remain in the primary navigation."
            kind="missing"
            title="Operator surface unavailable"
        />
    );
}
