# P29 Service Productization Entry Spec

- Status: In Progress — Contract Slice Complete

## Objective

matcher V0 conformance를 고정 입력으로 두고, `sema-journal-v1`과 HTTP `v0alpha1` prototype을 제품형 persistence와 API service boundary로 재설계한다. 첫 단계는 database나 transport를 성급히 고르는 것이 아니라 authority, transaction, resource와 migration contract를 executable failure scenario로 결정하는 것이다.

## Fixed Inputs

- matcher core와 public Go `alpha v0alpha5`는 `docs/matcher-conformance.md`를 통과하는 side-effect-free composition boundary다.
- proposal reserve/confirm/acknowledgment의 revision 및 roster CAS semantics는 보존한다.
- current checksummed journal, single-writer replay와 loopback HTTP는 migration source이자 V0 reference implementation이지 target architecture가 아니다.
- reusable candidate index의 incremental lifetime은 queue mutation과 transactional snapshot을 소유하는 demand repository에 연결한다.

## Decisions Before Adapter Implementation

1. queue ticket, backfill, policy, planning snapshot, proposal, reservation, assignment와 audit 각각의 durable source of truth 및 retention을 정한다.
2. plan/read snapshot과 reserve/commit 사이의 transaction/isolation boundary, idempotency key와 expiry clock authority를 정한다.
3. single-writer를 유지할지 multi-writer/replica coordination을 지원할지 failure scenario와 target workload로 결정한다.
4. versioned API resource identity, create/update/cancel semantics, pagination, polling/streaming delivery와 typed error envelope을 정한다.
5. authentication, tenant isolation, authorization, quota/rate limit과 redacted observability ownership을 정한다.
6. V0 journal/API를 in-place migrate, import-only 지원 또는 명시적으로 폐기할지 compatibility policy를 정한다.

## First Executable Slice

adapter-neutral repository contract와 service resource model을 먼저 만든다.

- 동일 ticket revision 경쟁, duplicate idempotency key, reservation expiry, process crash와 replay를 포함한 failure fixture를 작성한다.
- matcher는 immutable snapshot을 받고 repository는 snapshot/version과 mutation transaction을 소유한다.
- persistent transaction과 함께 갱신 가능한 candidate index lifecycle seam을 정의하되 specific database는 fixture와 benchmark 전까지 확정하지 않는다.
- V0 HTTP endpoint와 target resource model 사이의 migration mapping을 작성한다.

완료 evidence:

- `internal/repository`가 tenant-scoped resource CAS, operation receipt, redacted audit와 defensive snapshot 계약을 제공한다.
- `repositorytest.Run`을 in-memory adapter가 통과하며 same-version competition, atomic multi-resource conflict, duplicate operation과 reopen replay를 검증한다.
- `internal/service`가 target resource kind, repository-versioned planning snapshot과 stale candidate-index fence를 제공한다.
- ADR 0016과 `docs/service-authority.md`가 authority/retention, failure matrix, target operation과 V0 import-only mapping을 고정한다.

## Acceptance For The Next Milestone

- authority/transaction decision과 failure matrix가 architecture decision으로 승인된다.
- in-memory contract test와 최소 하나의 persistent adapter prototype이 같은 repository conformance suite를 통과한다.
- crash/retry/expiry 및 stale snapshot이 matcher conformance를 깨뜨리지 않는다.
- API schema가 idempotency, pagination/delivery와 error semantics를 명시하고 authentication 없는 remote exposure를 허용하지 않는다.
- migration/rollback과 operational ownership이 구현 전에 검증 가능하다.

현재 첫 번째, 세 번째 항목의 adapter-neutral 부분과 migration inventory는 완료되었다. 다음 implementation slice는 최소 하나의 persistent prototype을 같은 conformance에 연결하고 실제 process crash 전/후 replay fixture를 추가하는 것이다. storage product와 replica topology 채택은 그 evidence 뒤에 결정한다.

## Out Of Scope At Entry

- database vendor, message broker 또는 deployment topology의 근거 없는 확정.
- external production rollout, stable v1 선언이나 V0 data의 자동 파괴 migration.
- 실제 consumer corpus 없이 matcher quality policy를 다시 설계하는 일.

## Escalation Boundary

database/topology와 stable API compatibility는 cost, external dependency 또는 irreversible migration을 동반하므로 fixture/benchmark/failure evidence와 사용자 결정을 요구한다. 그 전까지 repository/service interface, contract tests와 migration inventory는 자율적으로 진행할 수 있다.
