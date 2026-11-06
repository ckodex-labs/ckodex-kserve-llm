# Gateway and scheduler kind verification — 2026-11-01

## Environment

The dedicated `ckodex-gpu-validation` cluster ran Kubernetes `v1.35.8` on
`192.168.2.198`. It used the Helm-installed CKodex manager, Envoy Gateway
`v1.8.4`, Envoy AI Gateway `v1.1.0`, Gateway API Inference Extension `v1.6.2`,
and llm-d Router CRDs `v1.0.0`. The test Gateway used a cert-manager-issued
self-signed certificate and a ClusterIP service accessed by loopback port-forward.
This is not a production trust or network-isolation configuration.

## Observed healthy path

With the scheduler enabled and requested, `SchedulerReady=True`, the HTTPRoute
referenced the `InferencePool`, and its parent reported `Accepted=True` and
`ResolvedRefs=True`. The EPP logged receipt of the inference request. A TLS
request with the model API key returned HTTP 200 and a nonempty GPU-generated
completion; the same request without the key returned HTTP 401.

## Observed scheduler-disabled failure

With the scheduler still requested but the operator's scheduler feature disabled,
the service reported `Ready=False` and `SchedulerReady=False` with reason
`SchedulerFeatureDisabled`. The HTTPRoute changed to the direct model Service,
and the same authenticated TLS inference request returned HTTP 200. This is a
failed fail-closed check; a false readiness condition did not prevent traffic.

As an isolated data-plane probe, a separate exact-path HTTPRoute with no
backends and no filters was accepted by Envoy Gateway. A verified-TLS request
to that path returned HTTP 500. This observation covers that HTTP route shape
and controller version. A separate no-backend, no-filter GRPCRoute was accepted;
a TLS gRPC client that verified the server certificate received non-success
status `UNKNOWN`. This
does not prove an inference gRPC backend, existing-connection handling, direct
Service access, third-party routes, or other Gateway implementations.

## Observed correction and recovery

After deploying the route-fencing change, the same scheduler-disabled case
removed all backends and filters from the owned HTTP and previously existing
gRPC routes. With the GPU model pod independently ready, both authenticated
and unauthenticated HTTPS inference attempts returned HTTP 500. Re-enabling
the scheduler restored the InferencePool backend, and authenticated inference
returned HTTP 200 while the unauthenticated request returned HTTP 401.

With the scheduler enabled but the operator Gateway feature disabled, the
existing HTTPRoute was fenced and both `SchedulerReady` and `Ready` reported
`False` with reason `GatewayFeatureDisabled`; HTTPS requests returned 500.
Re-enabling the Gateway feature restored inference through the InferencePool.

A separate same-name HTTPRoute without an owner reference was left unchanged;
the operator returned an ownership error instead of taking it over. This was a
real API-server test of the collision path, not proof about unrelated routes.

These are observed transitions, not a non-bypassable admission guarantee.
Paused resources are intentionally not reconciled; controller termination,
API failures and route propagation can delay fencing. Direct Service access,
foreign routes, and unsupported scheduled gRPC remain outside the proof.
