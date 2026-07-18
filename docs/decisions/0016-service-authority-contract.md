# ADR 0016: Service Authority And Repository Contract

- Status: Accepted

## Context

matcher V0는 immutable input에서 deterministic `ProposalBatch`를 만드는 계약을 닫았다. 반면 `internal/durable.Runtime`은 application state, file journal, idempotency index와 audit read model을 한 single-writer 구현에 결합한다. 이 구조는 V0 failure semantics를 검증했지만 persistent adapter, concurrent writer와 target API resource를 독립적으로 비교할 수 없다.

database나 deployment topology를 먼저 고르면 storage product의 transaction 제약이 service semantics를 역으로 결정할 위험이 있다. 먼저 모든 adapter가 통과해야 하는 authority와 transaction contract가 필요하다.

## Decision

### Durable Authority

- tenant-scoped repository가 policy, match/backfill ticket, planning snapshot/run, proposal, reservation, assignment, acknowledgment와 idempotent operation receipt의 durable authority다.
- matcher는 repository authority가 아니다. repository version에 묶인 immutable `MatchmakingSnapshot`만 받고 side effect 없는 결과를 반환한다.
- server clock이 planning time, reservation expiry와 lifecycle mutation time을 소유한다. client timestamp는 authority가 될 수 없다.
- external session authority가 assignment 적용과 resulting roster version을 소유한다. Sema는 그 결과를 acknowledgment로 기록하며 game execution을 소유하지 않는다.

### Transaction Contract

- repository read는 한 commit version의 defensive snapshot이다.
- 하나의 service command는 tenant scope 안에서 canonical payload digest를 가진 `operationID`, bounded command kind와 server time을 요구한다.
- mutation은 resource storage version을 compare-and-swap한다. 한 command의 모든 mutation, operation receipt와 redacted audit receipt는 하나의 commit version으로 함께 성공하거나 모두 실패한다.
- 같은 `operationID`와 digest의 retry는 최초 commit version을 반환하고 mutation을 다시 적용하지 않는다. 같은 ID의 다른 digest는 `IdempotencyConflict`다.
- planning input capture는 짧은 repository read/commit 경계에서 끝낸다. matcher search 중 storage transaction을 열어 두지 않는다.
- planning result는 immutable snapshot reference와 함께 저장한다. 이후 queue가 변해도 plan audit은 보존하지만 reserve transaction은 현재 ticket revision, roster version과 reservation availability를 다시 검증한다.
- repository storage conflict는 command context에 따라 `InvalidRevision`, `StaleSnapshot` 또는 `ReservationConflict`로 변환한다. storage adapter의 private 오류를 wire contract로 노출하지 않는다.

### Candidate Index

- reusable candidate index는 repository snapshot에서 재생성 가능한 derived state다.
- index는 자신이 반영한 repository version을 가진다. planning snapshot과 version이 다르면 사용할 수 없으며 owner가 commit delta를 반영하거나 snapshot에서 rebuild해야 한다.
- matcher 또는 index가 독립적으로 ticket freshness를 확정할 수 없다. reserve authority는 항상 repository transaction에 남는다.

### Writer Topology And Storage Product

- contract는 resource-level optimistic CAS를 사용하므로 single writer와 concurrent transaction adapter를 모두 시험할 수 있다.
- current journal의 single writer는 V0 reference behavior로 유지한다. target replica 수, database, broker와 lease topology는 persistent prototype conformance, contention benchmark와 recovery evidence 뒤에 결정한다.

### V0 Migration

- `sema-journal-v1`은 in-place rewrite하지 않는다. target repository로의 explicit import source로만 취급한다.
- importer는 V0 journal을 read-only로 열고 target repository가 비어 있을 때 새 operation/resource receipt를 생성한다. import 완료 표식 전 실패는 target을 폐기하고 다시 시작할 수 있어야 한다.
- HTTP `v0alpha1` route를 target resource semantics로 조용히 바꾸지 않는다. legacy surface와 새 versioned surface는 cutover 동안 분리하고 target write 시작 전까지 V0 rollback을 허용한다.
- automatic destructive migration과 stable compatibility 선언은 별도 승인 없이는 허용하지 않는다.

## Consequences

- `internal/repository`가 adapter-neutral CAS, idempotency, audit와 shared conformance suite를 제공하고 in-memory adapter가 reference implementation이 된다.
- `internal/service`가 tenant-scoped resource kind, repository-versioned planning snapshot과 candidate-index freshness fence를 소유한다.
- persistent adapter는 같은 conformance suite를 통과해야 하며 crash-before-commit과 crash-after-commit response loss를 구분해 검증해야 한다.
- file reference adapter와 subprocess crash/contention 결과는 `docs/repository-adapter-evidence.md`에 기록하고 product storage 선택과 분리한다.
- global serial execution이나 특정 SQL isolation 이름은 service contract가 아니다. adapter는 외부에서 관찰되는 atomicity와 conflict semantics를 만족하는 가장 작은 transaction을 사용한다.
- numeric retention, authentication provider, rate limit과 database/topology는 아직 선택하지 않는다. 그러나 authenticated tenant scope 없는 remote exposure는 target architecture에서 허용하지 않는다.

## Revisit Triggers

- persistent adapter prototype이 resource-level CAS 또는 atomic receipt를 제공하지 못한다.
- representative contention/load가 chosen transaction boundary의 retry budget을 초과한다.
- external consumer가 streaming delivery, transactional outbox 또는 cross-resource query isolation을 요구한다.
- regulatory deletion, audit retention 또는 tenant key-management 요구가 resource lifetime을 바꾼다.
