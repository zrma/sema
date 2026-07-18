# ADR 0017: PostgreSQL Authority Baseline

- Status: Accepted
- Date: 2026-07-18

## Context

ADR 0016은 tenant-scoped resource CAS, operation receipt와 audit receipt가 하나의 atomic commit이어야 한다고 고정했다. in-memory와 file reference adapter가 같은 contract, process crash와 reopen semantics를 검증했지만 file full-state rewrite와 single-process writer는 product service의 concurrent authority가 될 수 없다.

제품형 persistence에는 여러 stateless service replica가 같은 durable state를 안전하게 갱신하고, tenant별 snapshot/version, idempotency, pagination과 recovery를 제공할 write authority가 필요하다. Redis를 함께 넣으면 cache/coordination과 durable truth 사이의 invalidation 및 partial-failure contract가 하나 더 생기지만 현재 workload에는 이를 정당화하는 병목 evidence가 없다.

## Decision

- PostgreSQL primary를 target repository의 유일한 durable write authority로 채택한다.
- Sema service process는 stateless replica로 확장할 수 있으며 mutation correctness는 PostgreSQL transaction만 소유한다.
- write transaction은 Read Committed에서 operation ID를 tenant scope로 claim하고 resource row를 canonical order로 잠가 CAS를 확인한다.
- resource 준비가 끝난 뒤 tenant scope version row를 commit 직전에만 잠가 version, resource mutation, operation receipt와 audit receipt를 같은 순서로 확정한다.
- planning snapshot은 read-only Repeatable Read transaction에서 scope version과 resources를 함께 읽는다. matcher computation 중에는 database transaction을 유지하지 않는다.
- candidate index는 repository version에 연결된 rebuildable derived state이며 durable authority가 아니다.
- baseline에는 Redis, message broker, transactional outbox, cross-region multi-primary와 application lease owner를 넣지 않는다.
- schema migration은 service traffic 시작 전에 explicit step으로 실행한다. service startup이 암묵적으로 DDL을 수행하지 않는다.
- 기존 `sema-journal-v1` runtime과 HTTP `v0alpha1`은 target API cutover 전까지 V0 reference/import source로 유지한다.

## Consequences

- `internal/repository/postgres`가 `pgx/v5` pool과 repository-owned schema를 사용해 ADR 0016 contract를 구현한다.
- 서로 다른 tenant는 scope version lock을 공유하지 않는다. 같은 tenant의 unrelated resource transaction도 resource 준비는 병렬로 수행하고 version finalization만 짧게 직렬화한다.
- operation receipt unique constraint가 replica 사이의 동일 operation retry를 하나의 result로 수렴시킨다.
- PostgreSQL 장애나 commit outcome 불명확성은 같은 operation ID/digest retry로 복구한다. Redis failover 또는 cache recovery 절차는 존재하지 않는다.
- initial schema와 adapter는 internal pre-v1 contract다. provider, pool size, numeric timeout/SLO와 backup product는 deployment evidence 뒤 정한다.
- current `cmd/sema-server`가 자동으로 PostgreSQL을 사용하는 것은 아니다. authenticated target API와 import/cutover fixture가 준비될 때 runtime composition을 별도 변경한다.

## Verification

- in-memory/file/PostgreSQL adapter는 같은 `repositorytest.Run` contract를 통과한다.
- Docker-isolated PostgreSQL fixture가 separate pool reopen, same-version competition, atomic multi-resource conflict, idempotency replay, tombstone와 audit를 실행한다.
- 별도 pool의 unrelated resource commit이 tenant scope에서 서로 다른 연속 version을 받고 ordered audit에 나타난다.

## Revisit Triggers

- measured database polling이 planning latency 또는 primary load의 실제 병목이 된다.
- shared rate-limit counter나 ephemeral high-fanout delivery가 PostgreSQL/HTTP polling으로 감당되지 않는다.
- consumer가 transactional outbox, streaming delivery 또는 cross-region write availability를 요구한다.
- PostgreSQL transaction retry/conflict rate가 accepted workload budget을 초과한다.

Redis를 나중에 추가하더라도 reservation, assignment, operation receipt와 audit의 source of truth로 사용하지 않는다. Redis 장애는 성능이나 delivery freshness만 낮추고 correctness를 바꾸지 않아야 한다.
