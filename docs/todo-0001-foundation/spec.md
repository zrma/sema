# P0 Foundation Spec

## Objective

구현을 시작하기 전에 Sema의 canonical domain contract, lifecycle, deterministic reference scenarios, stack 선택 기준을 검증 가능한 형태로 고정한다.

## Deliverables

- `docs/domain-model.md`: entity field, identity, ownership, immutable/mutable boundary.
- `docs/lifecycle.md`: ticket, proposal, reservation, assignment state transition과 failure outcome.
- `docs/reference-scenarios.md`: new-match, party-preserving match, backfill, conflict/retry fixture.
- `docs/decisions/0001-implementation-stack.md`: workload와 검증 결과에 근거한 언어/runtime 선택.
- 선택한 stack의 최소 package skeleton과 deterministic fixture test.

## Acceptance

- 모든 entity는 stable ID, owner, version 또는 freshness contract를 가진다.
- hard constraint와 soft objective의 경계가 예제에서 혼동되지 않는다.
- new-match와 backfill이 같은 planner를 사용하면서도 서로 다른 lifecycle을 유지한다.
- 같은 fixture와 seed를 반복 실행하면 proposal ordering이 동일하다.
- stale roster와 duplicate reservation이 typed failure로 검증된다.
- 전체 `scripts/check.sh`가 선택한 언어 gate와 reference scenario를 실행한다.

## Execution Order

1. glossary와 entity schema를 문서화한다.
2. state transition과 failure semantics를 고정한다.
3. representative fixture와 performance envelope를 정의한다.
4. 후보 stack을 동일 기준으로 비교하고 decision record를 작성한다.
5. 최소 vertical slice와 deterministic test를 구현한다.
6. status/roadmap/check manifest를 실제 결과에 맞춘다.

## Out Of Scope

- distributed production deployment.
- public API compatibility guarantee.
- game-specific ranking formula.
- UI와 운영 dashboard.
- remote push, repository visibility, release.
