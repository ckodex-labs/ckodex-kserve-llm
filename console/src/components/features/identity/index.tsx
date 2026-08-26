import type { CapabilityDecision } from "@/lib/identity";
import type { AccessReviewSource, IdentityRegistrationSource } from "@/lib/identity.server";

function decisionLabel(decision: CapabilityDecision) {
    if (decision === "allowed") return "⊢ allowed";
    if (decision === "denied") return "⊭ denied";
    if (decision === "no-opinion") return "○ no opinion";
    return "○ unavailable";
}

function SourceBoundary({ detail }: { detail: string }) {
    return <p className="border border-dashed border-border p-5 text-sm leading-6 text-muted-foreground">{detail}</p>;
}

export function IdentityAuthorityBlock({
    registrations,
    access,
}: {
    registrations: IdentityRegistrationSource;
    access: AccessReviewSource;
}) {
    const principal = access.data.principal;
    const mutationGrants = access.data.capabilities.filter((item) => item.effect === "mutate" && item.decision === "allowed");

    return (
        <div>
            <section aria-labelledby="execution-principal-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Kubernetes self review</p>
                    <h2 className="mt-2 text-xl font-semibold" id="execution-principal-title">Execution principal</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Identity reported by the API server for console-originated requests. This service principal is distinct from an interactive human identity.
                    </p>
                </div>
                {principal ? (
                    <dl className="mt-px grid gap-px border border-border bg-border lg:grid-cols-[minmax(280px,1.4fr)_minmax(180px,0.7fr)_minmax(260px,1fr)]">
                        <div className="bg-background p-5">
                            <dt className="ck-label">Username</dt>
                            <dd className="mt-3 break-all font-mono text-xs">{principal.username}</dd>
                        </div>
                        <div className="bg-background p-5">
                            <dt className="ck-label">UID</dt>
                            <dd className="mt-3 break-all font-mono text-xs text-muted-foreground">{principal.uid || "Not reported"}</dd>
                        </div>
                        <div className="bg-background p-5">
                            <dt className="ck-label">Groups</dt>
                            <dd className="mt-3 break-words font-mono text-xs text-muted-foreground">
                                {principal.groups.length > 0 ? principal.groups.join(" · ") : "None reported"}
                            </dd>
                        </div>
                    </dl>
                ) : <SourceBoundary detail={access.detail} />}
            </section>

            <section className="mt-8" aria-labelledby="capability-matrix-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Effective access</p>
                    <h2 className="mt-2 text-xl font-semibold" id="capability-matrix-title">Capability matrix</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Non-mutating SelfSubjectAccessReview requests evaluate the console principal against a fixed operator capability set. A denied mutation is expected for observe mode.
                    </p>
                </div>
                <div aria-label="Capability matrix table" className="ck-table-scroll mt-px border border-border" role="region" tabIndex={0}>
                    <table className="w-full min-w-[820px] border-collapse text-left">
                        <thead className="bg-muted">
                            <tr>
                                <th className="ck-label p-4" scope="col">Capability</th>
                                <th className="ck-label p-4" scope="col">Effect</th>
                                <th className="ck-label p-4" scope="col">Resource</th>
                                <th className="ck-label p-4" scope="col">Decision</th>
                                <th className="ck-label p-4" scope="col">Reason</th>
                            </tr>
                        </thead>
                        <tbody>
                            {access.data.capabilities.length === 0 ? (
                                <tr><td className="border-t border-border p-6 text-sm text-muted-foreground" colSpan={5}>{access.detail}</td></tr>
                            ) : access.data.capabilities.map((capability) => (
                                <tr className="border-t border-border align-top" key={capability.id}>
                                    <td className="p-4 text-sm font-semibold">{capability.label}</td>
                                    <td className="p-4 font-mono text-xs uppercase text-muted-foreground">{capability.effect}</td>
                                    <td className="p-4 font-mono text-xs text-muted-foreground">
                                        {capability.verb} {capability.group ? `${capability.group}/` : "core/"}{capability.version}/{capability.resource}
                                        {capability.namespace ? ` · ns/${capability.namespace}` : ""}
                                    </td>
                                    <td className="p-4"><span className="ck-claim">{decisionLabel(capability.decision)}</span></td>
                                    <td className="max-w-[360px] p-4 text-xs leading-5 text-muted-foreground">
                                        {capability.reason}
                                        {capability.evaluationError ? <span className="mt-1 block font-mono">Evaluation: {capability.evaluationError}</span> : null}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
                {mutationGrants.length > 0 ? (
                    <p className="mt-px border border-foreground bg-foreground p-4 text-sm leading-6 text-background">
                        ⊭ Observe-mode mismatch: {mutationGrants.length} mutation grant{mutationGrants.length === 1 ? " is" : "s are"} active for the console principal.
                    </p>
                ) : null}
            </section>

            <section className="mt-8" aria-labelledby="registration-records-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Operator registration records</p>
                    <h2 className="mt-2 text-xl font-semibold" id="registration-records-title">SPIRE workload registrations</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        ConfigMaps written by the operator declare registration intent. They do not prove SVID issuance, current validity, workload attestation, or mTLS establishment.
                    </p>
                </div>
                <div className="mt-px border border-border">
                    {registrations.data.length === 0 ? (
                        <p className="p-6 text-sm leading-6 text-muted-foreground">{registrations.detail}</p>
                    ) : registrations.data.map((registration) => (
                        <article className="grid gap-5 border-b border-border p-5 last:border-b-0 xl:grid-cols-[minmax(280px,1.4fr)_minmax(220px,0.9fr)_minmax(180px,0.7fr)]" key={registration.id}>
                            <div>
                                <p className="ck-claim ck-claim--muted">○ registration observed</p>
                                <h3 className="mt-3 break-all font-mono text-sm font-semibold">{registration.spiffeId}</h3>
                                <p className="mt-2 font-mono text-[11px] text-muted-foreground">Trust domain {registration.trustDomain}</p>
                            </div>
                            <dl className="text-xs leading-5">
                                <div><dt className="ck-label inline">Workload</dt><dd className="ml-2 inline font-mono">{registration.sourceNamespace}/{registration.sourceService}</dd></div>
                                <div className="mt-2"><dt className="ck-label inline">Tenant</dt><dd className="ml-2 inline font-mono text-muted-foreground">{registration.tenantId || "Not reported"}</dd></div>
                                <div className="mt-2"><dt className="ck-label inline">TTL intent</dt><dd className="ml-2 inline font-mono text-muted-foreground">{registration.ttlSeconds === null ? "Not reported" : `${registration.ttlSeconds} seconds`}</dd></div>
                            </dl>
                            <div>
                                <p className="ck-label">Selectors</p>
                                <p className="mt-2 break-words font-mono text-[11px] leading-5 text-muted-foreground">{registration.selectors.join(" · ")}</p>
                                <p className="ck-label mt-4">Record</p>
                                <p className="mt-2 break-all font-mono text-[11px] text-muted-foreground">{registration.registrationNamespace}/{registration.configMapName}</p>
                            </div>
                        </article>
                    ))}
                </div>
            </section>

            <section className="mt-8" aria-labelledby="action-readiness-title">
                <div className="border border-border p-5">
                    <p className="ck-label">Mutation boundary</p>
                    <h2 className="mt-2 text-xl font-semibold" id="action-readiness-title">Governed action gate</h2>
                    <p className="mt-2 max-w-[72ch] text-sm leading-6 text-muted-foreground">
                        Action controls remain closed until a human principal, explicit policy decision, confirmation ceremony, and durable receipt sink are bound to the request.
                    </p>
                </div>
                <dl className="mt-px grid gap-px border border-border bg-border md:grid-cols-2">
                    <div className="bg-background p-5">
                        <dt className="ck-label">Human operator identity</dt>
                        <dd className="mt-3 font-mono text-xs">○ not bound</dd>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">The server-side service principal does not identify the interactive operator.</p>
                    </div>
                    <div className="bg-background p-5">
                        <dt className="ck-label">Execution principal</dt>
                        <dd className="mt-3 font-mono text-xs">{principal ? "⊢ observed" : "○ unavailable"}</dd>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">Kubernetes self-review qualifies the identity used for server-originated requests.</p>
                    </div>
                    <div className="bg-background p-5">
                        <dt className="ck-label">Observe-mode authority</dt>
                        <dd className="mt-3 font-mono text-xs">{mutationGrants.length === 0 ? "⊢ mutation withheld" : "⊭ privilege mismatch"}</dd>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">Mutation grants are expected to remain absent from the console ServiceAccount.</p>
                    </div>
                    <div className="bg-background p-5">
                        <dt className="ck-label">Action receipt</dt>
                        <dd className="mt-3 font-mono text-xs">○ sink not configured</dd>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">No signed action receipt or rollback record can be emitted by this surface.</p>
                    </div>
                </dl>
                <p className="mt-px border border-dashed border-border p-4 font-mono text-[11px] leading-5 text-muted-foreground">
                    ○ Gate closed · command references remain non-executing
                </p>
            </section>
        </div>
    );
}
