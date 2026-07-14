# P2 Assignment Lifecycle Spec

- Status: Active

## Objective

confirmed `Assignment`와 실제 game/session authority의 적용 결과를 idempotent하게 연결한다. matchmaker가 외부 roster를 직접 소유하지 않으면서 completion, cancellation, stale backfill CAS를 명시적 terminal outcome으로 기록한다.

## Contract Defaults

- assignment는 confirm 직후 `pending`이며 session/allocation consumer의 acknowledgment를 기다린다.
- `complete`와 `cancel`은 caller가 제공하는 opaque operation ID로 idempotent하다.
- terminal assignment에 다른 operation 또는 outcome을 적용하면 `IdempotencyConflict` 또는 `InvalidTransition`이다.
- assignment cancellation은 과거 ticket revision을 자동 복원하지 않는다. producer가 필요한 ticket을 새 revision과 enqueue semantics로 다시 제출한다.
- new-match completion은 외부 allocation 성공을 뜻하며 roster version을 요구하지 않는다.
- backfill completion은 proposal이 고정한 `sessionID`와 `rosterVersion`을 expected CAS 값으로 사용하고, session authority가 반환한 더 높은 `resultingRosterVersion`을 기록한다.
- roster CAS가 stale이면 assignment를 `failed` terminal outcome으로 기록하고 ticket을 자동 복원하지 않는다.

## Deliverables

- assignment status와 typed completion outcome domain schema.
- in-memory coordinator의 idempotent complete/cancel/fail transition.
- backfill expected/resulting roster version 검증.
- repeated operation, conflicting operation, stale roster, no-resurrection fixture.
- assignment outcome을 포함한 defensive-copy/read model.

## Acceptance

- 같은 operation ID와 payload의 complete/cancel 반복은 동일 assignment outcome을 반환한다.
- 같은 operation ID의 다른 payload와 terminal outcome 변경은 typed failure다.
- backfill completion은 session ID, expected roster version, strictly higher resulting roster version을 검증한다.
- stale backfill acknowledgment는 `StaleSnapshot` failure outcome으로 남아 재시작 없이 조회할 수 있다.
- cancellation/failure 뒤 과거 match ticket이 coordinator snapshot에 자동으로 나타나지 않는다.
- concurrent terminal transition은 정확히 하나만 성공한다.
- 전체 race detector와 lifecycle/reference gate가 통과한다.

## Out Of Scope

- allocation server와 session roster 저장소 구현.
- event delivery guarantee와 durable outbox.
- automatic ticket requeue policy.
- assignment timeout과 reconciliation worker.
- distributed coordinator와 process restart recovery.
