# Repository Adapter Evidence

## Purpose

P29는 database 제품을 먼저 고르지 않고 모든 target adapter가 만족해야 하는 transaction 의미론을 고정했다. `internal/repository/fileprototype`은 그 contract가 process crash와 reopen 뒤에도 구현 가능한지 확인하는 persistent reference adapter다. 이 package는 storage 제품 후보가 아니며 production data에 사용하지 않는다.

## Executable Evidence

in-memory adapter와 file prototype은 같은 `repositorytest.Run` suite를 실행한다.

- tenant별 defensive snapshot과 monotonically increasing repository version.
- resource별 compare-and-swap 및 여러 resource mutation의 all-or-nothing commit.
- mutation, operation receipt와 redacted audit receipt의 동일 commit version.
- 같은 operation ID/digest replay와 다른 digest의 idempotency conflict.
- reopen 뒤 resource, tombstone, operation receipt와 audit replay.
- 같은 resource를 갱신하는 concurrent caller의 single-winner CAS.

file prototype은 각 commit에서 checksummed full-state snapshot을 private temporary file에 기록하고 sync한 뒤 atomic rename과 directory sync를 수행한다. 실제 subprocess fixture가 다음 경계를 검증한다.

| Crash boundary | Reopen outcome | Same operation retry |
|---|---|---|
| temporary file sync 뒤, rename 전 | 이전 resource와 receipt만 보임 | 새 commit으로 실행 |
| rename과 directory sync 뒤, response 전 | 새 resource와 receipt가 함께 보임 | 최초 commit version을 replay |

checksum mismatch, 알 수 없는 schema/field와 private mode가 아닌 state file은 reopen 때 거부한다. 이 검증은 application-level atomicity와 corruption refusal evidence이며 filesystem, disk controller 또는 backup product 전체의 durability 보증은 아니다.

## Comparative Workload

`repositorytest.BenchmarkAdapter`는 두 adapter에 같은 sequential mutation과 same-resource contended CAS/retry workload를 적용한다. bounded 100-operation local comparison에서는 memory baseline이 sequential commit당 sub-microsecond인 반면 full-state file prototype은 약 12 ms로 네 자릿수 배 이상 느렸다. contended path도 file prototype이 약 12 ms로, memory baseline보다 수천 배 느리고 operation당 CAS retry도 더 많았다.

이 수치는 product SLO나 hardware-independent target이 아니다. 다만 operation receipt와 audit history가 늘어날수록 매 commit 전체 state를 encode, sync, replace하는 비용이 함께 증가한다는 구조적 차이를 보여준다. 따라서 이 prototype의 성능을 튜닝하거나 production adapter로 승격하지 않는다.

재현 명령은 다음과 같다.

```sh
go test -race ./internal/repository/...
go test ./internal/repository -run '^$' -bench '^BenchmarkMemoryRepository$' -benchtime=100x -count=3
go test ./internal/repository/fileprototype -run '^$' -bench '^BenchmarkFilePrototypeRepository$' -benchtime=100x -count=3
```

## Decision Evidence

현재 contract에서 필요한 capability는 다음과 같다.

- 여러 resource row, operation receipt와 audit receipt를 묶는 짧은 atomic transaction.
- `(tenant, operation_id)` uniqueness와 digest replay 판정.
- resource storage version을 조건으로 하는 compare-and-swap update.
- unrelated tenant/queue ingress가 한 global file writer에 막히지 않는 concurrent transaction.
- tenant-scoped stable pagination, retention, backup과 operational recovery.
- matcher search는 transaction 밖에서 실행하고 reserve 때 참조 resource만 다시 검증하는 경계.

file/embedded single-node adapter는 conformance를 구현할 수 있지만 multi-process writer, failover와 operational backup 요구가 생기면 별도 coordination을 다시 설계해야 한다. 반면 PostgreSQL은 위 transaction, uniqueness, row-level concurrency와 operational recovery를 직접 표현할 수 있으므로 첫 product adapter의 권장안이다.

## Recommended Initial Topology

사용자 승인 시 다음 milestone은 PostgreSQL-backed adapter를 대상으로 한다.

- 하나의 PostgreSQL primary를 durable write authority로 둔다.
- service process는 stateless replica로 늘릴 수 있지만 repository transaction만 mutation authority가 된다.
- operation receipt의 tenant-scoped unique insert, resource CAS, audit insert를 하나의 짧은 transaction으로 묶는다.
- matcher computation 중 transaction을 유지하지 않는다.
- candidate index는 commit version을 따라가는 rebuildable derived state로 유지한다.
- 첫 adapter에는 broker, transactional outbox, cross-region multi-primary, application lease owner를 넣지 않는다.

이 선택은 PostgreSQL schema, deployment vendor, replica 수, numeric SLO 또는 stable API를 아직 확정하지 않는다. 그 항목은 실제 adapter fixture와 consumer workload를 통해 별도로 좁힌다.

## Decision Outcome

P29의 persistent conformance, real process crash와 comparative contention evidence를 바탕으로 PostgreSQL primary를 첫 target persistent adapter와 write authority로 채택했다. service는 stateless replica로 확장하고 Redis는 baseline에서 제외한다. 실제 schema, transaction과 conformance는 `docs/postgres-repository.md`와 ADR 0017이 소유한다.
