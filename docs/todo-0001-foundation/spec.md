# P0 Foundation Spec

## Objective

Sema의 canonical domain contract와 lifecycle을 고정하고, 확정된 Go 단일 프로세스·인메모리 baseline에서 deterministic reference scenarios를 실행 가능한 형태로 검증한다.

## Accepted Inputs

- 대표 workload와 party distribution: `docs/reference-workloads.md`.
- implementation baseline: `docs/decisions/0001-implementation-baseline.md`.
- consistency 기본값과 남은 policy 질문: `open-questions.md`.

## Deliverables

- `docs/domain-model.md`: entity field, identity, ownership, immutable/mutable boundary.
- `docs/lifecycle.md`: ticket, proposal batch, reservation, assignment state transition과 failure outcome.
- `docs/reference-scenarios.md`: new-match, party-preserving match, backfill, conflict/retry fixture.
- Go module과 package boundary를 갖춘 최소 vertical slice.
- deterministic `ProposalBatch`, stale revision, reservation conflict test.

## Acceptance

- 모든 entity는 stable ID, owner, version 또는 freshness contract를 가진다.
- hard constraint와 time-dependent soft objective의 경계가 예제에서 혼동되지 않는다.
- new-match와 backfill이 같은 planner를 사용하면서도 서로 다른 lifecycle을 유지한다.
- 한 `ProposalBatch` 안에서 같은 ticket은 최대 한 proposal에만 나타난다.
- 같은 fixture와 seed를 반복 실행하면 proposal ordering이 동일하다.
- stale roster와 duplicate reservation이 typed failure로 검증된다.
- 전체 `scripts/check.sh`가 Go gate와 reference scenario를 실행한다.

## Execution Order

1. glossary와 entity schema를 문서화한다.
2. state transition과 revision/CAS failure semantics를 고정한다.
3. representative fixture와 performance envelope를 정의한다.
4. Go package skeleton과 인메모리 repository를 만든다.
5. multi-match planner와 coordinator의 최소 vertical slice를 구현한다.
6. deterministic, stale snapshot, reservation conflict test를 추가한다.
7. status/roadmap/check manifest를 실제 결과에 맞춘다.

## Out Of Scope

- process 분리와 distributed production deployment.
- durable persistence와 process restart recovery.
- public API compatibility guarantee.
- game-specific ranking formula의 최종 수치.
- UI와 운영 dashboard.
- remote push, repository visibility, release.
