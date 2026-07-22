# PostgreSQL Repository

## Scope

`internal/repository/postgres`는 P29 target persistence의 첫 product adapter다. PostgreSQL primary가 tenant-scoped resource, operation receipt와 redacted audit의 durable authority이며 Redis는 사용하지 않는다.

이 adapter와 `cmd/sema-target-server`의 존재는 현재 V0 `cmd/sema-server` writer cutover를 의미하지 않는다. local backup/restore와 pre-writer V0 rollback rehearsal 및 별도 authenticated listener는 완료했지만 기존 journal/HTTP service는 deployment-specific identity/TLS/E2E acceptance까지 reference surface로 남는다.

## Schema Ownership

repository가 소유하는 schema는 `internal/repository/postgres/schema.sql`이다.

- `sema_repository_metadata`: schema version.
- `sema_repository_scopes`: tenant scope별 ordered repository version.
- `sema_repository_resources`: tombstone을 포함한 current resource row.
- `sema_repository_operations`: tenant-scoped idempotency receipt.
- `sema_repository_audit`: operation kind, time과 resource-kind count만 가진 redacted receipt.

`postgres.Migrate`는 schema를 idempotent하게 설치하고 exact schema version을 확인한다. `cmd/sema-postgres-migrate`가 이를 pre-traffic Job에서 실행하며 service startup은 자동 호출하지 않는다. schema 변경은 versioned migration과 rollback/cutover evidence 없이 기존 statement를 조용히 바꾸지 않는다.

## Transaction Boundary

write path는 다음 순서를 사용한다.

1. operation scope를 확인하고 `(scope, operation_id)`를 unique receipt로 claim한다.
2. mutation resource를 canonical key order로 `FOR UPDATE`하고 expected storage version을 검증한다.
3. mutation 준비가 끝난 뒤 scope version row를 `FOR UPDATE`해 다음 version을 배정한다.
4. resource mutation, finalized operation receipt, audit receipt와 scope version을 같은 transaction에서 기록한다.
5. commit response가 유실되면 같은 operation ID/digest로 retry해 최초 version을 replay한다.

scope version lock은 commit ordering과 lossless audit cursor를 위해 필요하지만 matcher search나 전체 request lifetime 동안 유지하지 않는다. snapshot은 read-only Repeatable Read transaction에서 scope version과 resource set을 함께 읽고 transaction을 닫은 뒤 matcher에 전달한다.

## Concurrency And Failure

- 같은 resource/version 경쟁은 한 transaction만 성공한다.
- 여러 resource 중 하나가 stale이면 receipt와 모든 mutation이 rollback된다.
- 서로 다른 service pool은 같은 operation ID를 하나의 receipt로 수렴시킨다.
- 서로 다른 tenant는 version lock을 공유하지 않는다.
- 같은 tenant의 unrelated resource commit은 final version 배정만 짧게 직렬화한다.
- PostgreSQL commit 결과가 불명확하면 operation ID를 바꾸지 않고 retry한다.
- candidate index는 exact snapshot version이 아니면 사용하지 않고 rebuild한다.

## Local Verification

일반 Go gate는 external database 없이 adapter를 compile하고 integration test를 skip한다. 실제 PostgreSQL contract는 격리된 pinned container에서 실행한다.

```sh
scripts/check-postgres.sh
```

외부 PostgreSQL에 직접 연결해 test를 실행할 때는 test 전용 database만 사용한다.

```sh
SEMA_POSTGRES_TEST_DSN='<test-dsn>' go test -race ./internal/repository/postgres
```

test는 매 fixture마다 별도 schema를 만들고 종료 시 제거한다. 같은 script의 cutover rehearsal은 stopped V0 fixture를 import한 isolated schema를 `pg_dump`한 뒤 schema를 삭제하고 `pg_restore`한다. 복원 전후 resource/audit digest와 repository table row count, completion marker와 terminal assignment가 같아야 하며, 복원 target을 폐기한 다음 V0 source를 digest 변화 없이 다시 연다. production database나 shared user data를 대상으로 실행하지 않는다.

## Operational Boundary

- credential, TLS root와 provider endpoint는 repository에 기록하지 않는다.
- connection pool size, statement timeout와 retry budget은 representative authenticated workload 뒤 정한다.
- local logical backup/restore fixture는 ADR 0024가 소유한다. production backup/PITR, migration runner와 deployment topology는 provider-specific operations milestone이 소유한다.
- Redis, broker와 outbox는 measured trigger 없이 추가하지 않는다.
