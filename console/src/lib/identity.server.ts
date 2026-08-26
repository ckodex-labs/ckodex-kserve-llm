import "server-only";

import * as k8s from "@kubernetes/client-node";

import {
    capabilityDecision,
    consoleCapabilitySpecs,
    parseSpireRegistration,
    spireRegistrationLabel,
    type CapabilityResult,
    type PrincipalIdentity,
    type SpireRegistration,
} from "./identity";
import { kubernetesFailurePhrase, kubernetesRequestOptions, kubernetesRequestTimeoutMs } from "./kubernetes-request";
import type { SourceSnapshot } from "./source";

export type IdentityRegistrationSource = SourceSnapshot<SpireRegistration[]>;

export interface AccessReviewData {
    principal: PrincipalIdentity | null;
    capabilities: CapabilityResult[];
}

export type AccessReviewSource = SourceSnapshot<AccessReviewData>;

function registrationNamespace() {
    const configured = process.env.CKODEX_SPIRE_REGISTRATION_NAMESPACE?.trim();
    return configured || "spire";
}

export async function getIdentityRegistrationSource(): Promise<IdentityRegistrationSource> {
    const namespace = registrationNamespace();
    const source = `Kubernetes core/v1 ConfigMaps · ns/${namespace}`;
    const timeoutMs = kubernetesRequestTimeoutMs();

    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const coreApi = kc.makeApiClient(k8s.CoreV1Api);
        const response = await coreApi.listNamespacedConfigMap(
            { namespace, labelSelector: spireRegistrationLabel },
            kubernetesRequestOptions(timeoutMs),
        );

        const registrations: SpireRegistration[] = [];
        let malformed = 0;
        for (const configMap of response.items) {
            const registration = parseSpireRegistration(configMap);
            if (registration) registrations.push(registration);
            else malformed += 1;
        }
        registrations.sort((left, right) => left.spiffeId.localeCompare(right.spiffeId));

        return {
            state: malformed > 0 ? (registrations.length > 0 ? "partial" : "malformed") : registrations.length > 0 ? "observed" : "empty",
            data: registrations,
            source,
            observedAt: new Date().toISOString(),
            detail: malformed > 0
                ? `${registrations.length} registration record${registrations.length === 1 ? "" : "s"} parsed; ${malformed} malformed record${malformed === 1 ? "" : "s"} withheld.`
                : `${registrations.length} SPIRE registration record${registrations.length === 1 ? "" : "s"} returned. Registration does not prove SVID issuance or validity.`,
        };
    } catch (error) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`Kubernetes SPIRE registration source unavailable: ${failure}.`);
        return {
            state: "unavailable",
            data: [],
            source,
            observedAt: null,
            detail: `The registration ConfigMap request ${failure}. Check endpoint reachability, the active context, namespace, and ConfigMap list permission.`,
        };
    }
}

export async function getAccessReviewSource(): Promise<AccessReviewSource> {
    const source = "Kubernetes authentication.k8s.io/v1 + authorization.k8s.io/v1 self reviews";
    const emptyData: AccessReviewData = { principal: null, capabilities: [] };
    const timeoutMs = kubernetesRequestTimeoutMs();
    const requestOptions = kubernetesRequestOptions(timeoutMs);

    try {
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        const authenticationApi = kc.makeApiClient(k8s.AuthenticationV1Api);
        const authorizationApi = kc.makeApiClient(k8s.AuthorizationV1Api);
        const capabilitiesToReview = consoleCapabilitySpecs.map((capability) => capability.id === "spire-registration-list"
            ? { ...capability, namespace: registrationNamespace() }
            : capability);

        const principalPromise = authenticationApi.createSelfSubjectReview({
            body: { apiVersion: "authentication.k8s.io/v1", kind: "SelfSubjectReview" },
        }, requestOptions);
        const reviewPromises = capabilitiesToReview.map((capability) => authorizationApi.createSelfSubjectAccessReview({
            body: {
                apiVersion: "authorization.k8s.io/v1",
                kind: "SelfSubjectAccessReview",
                spec: {
                    resourceAttributes: {
                        group: capability.group,
                        version: capability.version,
                        resource: capability.resource,
                        verb: capability.verb,
                        ...(capability.namespace ? { namespace: capability.namespace } : {}),
                    },
                },
            },
        }, requestOptions));

        const [principalResult, ...reviewResults] = await Promise.allSettled([principalPromise, ...reviewPromises]);
        const principalInfo = principalResult.status === "fulfilled" ? principalResult.value.status?.userInfo : null;
        const principal = principalInfo?.username ? {
            username: principalInfo.username,
            uid: principalInfo.uid || null,
            groups: [...(principalInfo.groups || [])].sort(),
        } : null;

        const capabilities = capabilitiesToReview.map((capability, index): CapabilityResult => {
            const result = reviewResults[index];
            if (!result || result.status === "rejected") {
                return {
                    ...capability,
                    decision: "unavailable",
                    reason: result?.status === "rejected"
                        ? `Review request ${kubernetesFailurePhrase(result.reason, timeoutMs)}.`
                        : "Review response missing.",
                    evaluationError: null,
                };
            }

            const status = result.value.status;
            return {
                ...capability,
                decision: status ? capabilityDecision(status.allowed, status.denied) : "unavailable",
                reason: status?.reason || "No authorization reason returned.",
                evaluationError: status?.evaluationError || null,
            };
        });

        const failed = capabilities.filter((item) => item.decision === "unavailable").length + (principal ? 0 : 1);
        const returned = capabilities.length - capabilities.filter((item) => item.decision === "unavailable").length;
        return {
            state: failed === capabilities.length + 1 ? "unavailable" : failed > 0 ? "partial" : "observed",
            data: { principal, capabilities },
            source,
            observedAt: returned > 0 || principal ? new Date().toISOString() : null,
            detail: `${returned} of ${capabilities.length} capability reviews returned${principal ? "; execution principal observed." : "; execution principal unavailable."}`,
        };
    } catch (error) {
        const failure = kubernetesFailurePhrase(error, timeoutMs);
        console.warn(`Kubernetes self-review source unavailable: ${failure}.`);
        return {
            state: "unavailable",
            data: emptyData,
            source,
            observedAt: null,
            detail: `The Kubernetes self-review source ${failure}. Check endpoint reachability, the active context, and API availability.`,
        };
    }
}
