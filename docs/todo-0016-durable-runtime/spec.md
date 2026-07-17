# P9 Durable Runtime Foundation Spec

- Status: Complete

## Objective

single-replica engine의 accepted state와 decision audit을 restart 뒤 복구하고 idempotent retry 범위를 process lifetime 밖으로 확장한다.

## Deliverables

- checksummed, sequenced `sema-journal-v1` append-only store.
- file sync completion boundary와 failed-append in-memory rollback/replay.
- fixed reservation TTL configuration record.
- policy, ticket, plan, reservation, assignment와 acknowledgment replay.
- complete plan audit, snapshot ID idempotency와 paged defensive read.
- Darwin/Linux single-writer lock과 private file permissions.

## Acceptance

- active reservation이 restart 뒤 ticket을 계속 소유하고 confirm할 수 있다.
- confirmed/terminal assignment와 same-ID retry가 restart 뒤 동일하다.
- complete plan batch와 unmatched digest가 ordered audit에 남는다.
- torn final tail은 복구하고 complete checksum/schema corruption은 startup failure다.
- concurrent second writer와 TTL drift를 거부한다.
- focused test/race와 102/1002-event replay benchmark가 실행된다.

## Out Of Scope

- HTTP/gRPC ingestion과 assignment delivery endpoint.
- multi-replica writer, external database와 distributed lock.
- journal compaction, online backup와 encryption key management.
- numeric startup/recovery SLO.

## Completion Evidence

`go test ./internal/durable`, race detector와 `BenchmarkOpenReplay`가 통과하고 전체 repository gate에 편입된다. durable contract는 `docs/durable-runtime.md`, architecture decision은 ADR 0010이 소유한다.
