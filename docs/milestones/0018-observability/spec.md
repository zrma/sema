# P10 Observability And Audit Export Spec

- Status: Complete

## Objective

service health, request volume/latency/failure와 durable decision history를 private identity leakage 없이 측정 가능하게 만든다.

## Deliverables

- liveness/readiness와 Prometheus metrics endpoint.
- W3C traceparent propagation과 redacted JSON request span.
- event-specific redacted audit summary와 paged API.
- bounded route/status/failure metric labels와 fixed latency buckets.

## Acceptance

- trace ID를 이어받고 response span context를 반환한다.
- metric label과 trace route에 concrete resource ID가 없다.
- audit output에 raw journal payload, ticket/player/snapshot/reservation ID가 없다.
- ready endpoint가 durable runtime state를 확인한다.
- concurrent metric update와 audit defensive read가 race detector를 통과한다.
- full repository/publication gate가 통과한다.

## Out Of Scope

- OTLP/Prometheus deployment와 dashboard/alert provisioning.
- per-ticket public diagnostics와 raw journal download.
- durable metric retention과 cross-replica aggregation.

## Completion Evidence

`go test ./internal/observability ./internal/httpapi ./internal/durable`와 race detector가 통과한다. contract는 `docs/observability.md`, decision은 ADR 0012가 소유한다.
