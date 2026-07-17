# ADR 0002: Same-Process Engine Adapter With Producer Replay

- Status: Superseded by ADR 0011
- Decision date: 2026-07-15

## Context

`internal/engine`은 ingestion, planning, reservation, assignment와 acknowledgment를 하나의 transport-neutral Go boundary로 제공한다. 아직 독립 consumer process, durable event delivery, multi-replica coordinator 또는 public API compatibility 요구는 없다. 이 상태에서 forwarding adapter나 protocol을 추가하면 domain semantics를 검증하기 전에 retry와 storage 계약을 임의로 고정하게 된다.

## Decision

- 첫 consumer는 같은 Go process에서 `internal/engine.Engine`을 직접 호출한다.
- engine의 domain input과 typed output을 별도 DTO 또는 failure mapping 없이 사용한다.
- caller가 time과 snapshot, reservation, assignment, operation ID를 제공한다.
- assignment acknowledgment는 direct synchronous call로 처리한다.
- idempotency와 retry guarantee는 하나의 process lifetime에 한정한다.
- process restart 뒤 engine은 empty state로 시작하고 producer가 active match/backfill demand 전체를 다시 제출한다.
- 첫 deployment는 single replica이며 public module/API compatibility는 고정하지 않는다.

## Consequences

- planner와 coordinator semantics를 protocol이나 database 없이 실행하고 측정할 수 있다.
- network timeout, message ordering, outbox/inbox와 schema migration을 아직 설계하지 않는다.
- restart 시 active reservation과 assignment read model을 복구하지 않는다.
- producer가 active demand를 재제출할 수 없거나 assignment recovery가 필요해지면 이 adapter는 충분하지 않다.

## Extraction Triggers

다음 중 하나가 실제 integration evidence로 확인되면 transport와 persistence를 함께 재설계한다.

- consumer가 별도 process 또는 언어에서 호출해야 한다.
- producer가 전체 active demand를 재제출할 수 없다.
- acknowledgment delivery가 process failure를 넘어 재시도되어야 한다.
- multi-replica coordinator 또는 independent scaling이 필요하다.
- public API compatibility와 versioned schema가 필요하다.

adapter를 분리할 때는 delivery guarantee, idempotency scope, timeout, retry, durable CAS source of truth를 하나의 decision으로 고정한다.
