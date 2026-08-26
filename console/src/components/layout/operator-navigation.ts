import type { LucideIcon } from "lucide-react";
import {
    Activity,
    Box,
    Cpu,
    Database,
    LayoutDashboard,
    ScrollText,
    Shield,
    Terminal,
    Zap,
} from "lucide-react";

import type { DeploymentProfile } from "@/lib/config";

export type DashboardNavigationMessageKey =
    | "infrastructureOverview"
    | "inferenceFleet"
    | "fabricTerminal"
    | "auditEvents"
    | "identityVault"
    | "telemetry"
    | "secureStorage"
    | "nodeManager"
    | "matrixAccel";

export interface OperatorNavigationItem {
    titleKey: DashboardNavigationMessageKey;
    icon: LucideIcon;
    tooltip: string;
    url: string;
    enabled: boolean;
    keywords: string;
}

export interface OperatorNavigationGroup {
    label: string;
    items: OperatorNavigationItem[];
}

export function getOperatorNavigation(profile: DeploymentProfile): OperatorNavigationGroup[] {
    return [
        {
            label: "Model serving",
            items: [
                { titleKey: "infrastructureOverview", icon: LayoutDashboard, tooltip: "Reconciliation overview", url: "/", enabled: true, keywords: "status desired observed ready attention" },
                { titleKey: "inferenceFleet", icon: Zap, tooltip: "Inference services", url: "/fleet", enabled: profile.features.fleet, keywords: "models workloads llminferenceservice readiness" },
                { titleKey: "fabricTerminal", icon: Terminal, tooltip: "Reviewed command reference", url: "/terminal", enabled: profile.features.terminal, keywords: "kubectl dagger command copy" },
            ],
        },
        {
            label: "Governance",
            items: [
                { titleKey: "auditEvents", icon: ScrollText, tooltip: "Operator event ledger", url: "/events", enabled: profile.features.audit, keywords: "audit receipt actor outcome execution" },
                { titleKey: "identityVault", icon: Shield, tooltip: "Execution identity and effective access", url: "/identity", enabled: profile.features.identity, keywords: "spire svid rbac access principal identity" },
                { titleKey: "telemetry", icon: Activity, tooltip: "Metrics and active alerts", url: "/telemetry", enabled: profile.features.telemetry, keywords: "prometheus metrics alerts trends" },
                { titleKey: "secureStorage", icon: Database, tooltip: "Storage resources", url: "/storage", enabled: profile.features.storage, keywords: "storage cache artifacts" },
            ],
        },
        {
            label: "Infrastructure",
            items: [
                { titleKey: "nodeManager", icon: Box, tooltip: "Kubernetes nodes", url: "/nodes", enabled: true, keywords: "nodes capacity readiness cpu memory" },
                { titleKey: "matrixAccel", icon: Cpu, tooltip: "Accelerator inventory", url: "/accelerators", enabled: true, keywords: "gpu tpu accelerator allocatable" },
            ],
        },
    ];
}
