export interface DeploymentProfile {
    latticeName: string;
    environment: 'production' | 'staging' | 'development' | 'sandbox';
    features: {
        fleet: boolean;
        terminal: boolean;
        audit: boolean;
        identity: boolean;
        telemetry: boolean;
        storage: boolean;
        istio: boolean;
        keda: boolean;
    };
    branding: {
        primaryColor?: string;
        logoUrl?: string;
    };
}

const deploymentEnvironments: DeploymentProfile["environment"][] = [
    "production",
    "staging",
    "development",
    "sandbox",
];

function resolveEnvironment(): DeploymentProfile["environment"] {
    const configured = process.env.NEXT_PUBLIC_ENVIRONMENT;
    return deploymentEnvironments.find((environment) => environment === configured) ?? "development";
}

const DEFAULT_PROFILE: DeploymentProfile = {
    latticeName: process.env.NEXT_PUBLIC_LATTICE_NAME || "CKodex Lattice",
    environment: resolveEnvironment(),
    features: {
        fleet: true,
        terminal: process.env.NEXT_PUBLIC_FEATURE_TERMINAL !== 'false',
        audit: true,
        identity: true,
        telemetry: true,
        storage: false,   // Pending implementation
        istio: false,     // Only if explicitly required
        keda: false,      // Only if explicitly required
    },
    branding: {}
};

export function resolveDeploymentProfile(): DeploymentProfile {
    return DEFAULT_PROFILE;
}
