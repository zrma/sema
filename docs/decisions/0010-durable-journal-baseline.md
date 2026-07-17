# ADR 0010: Single-Writer Durable Journal Baseline

- Status: Accepted

## Context

in-memory engine은 deterministic replay와 process-local idempotency를 제공하지만 active reservation과 assignment를 재시작 뒤 잃는다. P9는 이를 복구해야 하지만 아직 multi-replica database, external queue나 stable service schema를 선택할 consumer evidence는 없다.

## Decision

- 기존 engine semantics를 바꾸지 않고 `internal/durable.Runtime`이 모든 stateful call을 직렬화한다.
- checksummed, monotonic `sema-journal-v1` JSON Lines file을 durable state와 decision audit의 source of truth로 사용한다.
- 성공은 complete record append와 file sync 뒤에만 반환한다.
- startup은 journal을 처음부터 replay하며 torn final tail만 복구하고 complete corruption은 거부한다.
- reservation TTL은 첫 configuration record에 고정한다.
- OS file lock으로 Darwin/Linux의 single writer를 강제한다.
- plan audit은 complete batch와 unmatched digest를 보존하고 snapshot ID retry에 최초 결과를 반환한다.
- process-local engine과 durable wrapper는 같은 deployable 안에 유지한다.

## Consequences

- reservation, assignment와 acknowledgment idempotency가 restart를 넘어 유지된다.
- 별도 database 없이 recovery/failure contract를 실행 가능하게 검증할 수 있다.
- append-only replay cost와 disk growth는 event 수에 비례한다.
- journal payload schema 변경에는 explicit migration 또는 새 schema가 필요하다.
- file journal은 horizontal writer coordination을 제공하지 않는다.

## Revisit Triggers

- measured replay startup 또는 disk-growth SLO가 compaction/snapshot을 요구한다.
- multi-replica write authority나 remote durable storage가 필요하다.
- online backup, encryption at rest, retention 또는 deletion policy가 필요하다.
- service delivery가 transactionally coupled outbox를 요구한다.
