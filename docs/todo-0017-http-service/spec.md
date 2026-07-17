# P9 Versioned HTTP Service Spec

- Status: Complete

## Objective

durable single-writer runtime을 별도 process consumer가 안전하게 호출하고 restart-safe assignment delivery/retry를 실행할 수 있는 experimental HTTP boundary를 제공한다.

## Deliverables

- explicit `internal/api/v0alpha1` request/response DTO와 envelope.
- policy, match/backfill ingestion, plan, reservation, assignment polling/ack endpoints.
- server-owned clock과 authoritative durable proposal lookup.
- typed HTTP failure/retry mapping, strict JSON와 bounded body.
- loopback-default `cmd/sema-server`, fixed timeout와 graceful shutdown.

## Acceptance

- domain struct나 internal proposal body를 wire authority로 사용하지 않는다.
- plan 직후 restart해도 proposal ID reserve가 성공한다.
- confirm 직후 restart해도 assignment poll과 same-operation acknowledgment retry가 동일하다.
- malformed, oversized, path mismatch, missing resource와 forged proposal request가 typed error다.
- backfill upsert/cancel freshness endpoint가 durable runtime을 호출한다.
- non-loopback listener는 explicit unsafe flag 없이 기동하지 않는다.
- focused test/race, command start/stop와 real loopback smoke가 통과한다.

## Out Of Scope

- authentication, authorization, TLS termination과 rate limiting.
- multi-replica writer, load balancer와 external database.
- push assignment delivery, outbox worker와 consumer registry.
- stable/v1 schema와 generated client SDK.

## Completion Evidence

`go test ./internal/httpapi ./cmd/sema-server`, race detector와 repository full gate가 통과한다. service contract는 `docs/service-api.md`, architecture decision은 ADR 0011이 소유한다.
