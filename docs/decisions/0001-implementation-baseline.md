# ADR 0001: Go Modular Monolith With In-Memory State

- Status: Accepted
- Decision date: 2026-07-15

## Context

Sema는 deterministic planner와 stateful coordinator를 모두 필요로 하지만 아직 production workload, 독립 scaling, durable recovery 요구가 검증되지 않았다. 초기부터 process와 storage를 분리하면 domain semantics보다 transport와 coordination 비용을 먼저 떠안게 된다.

## Decision

- 구현 언어는 Go를 사용한다.
- 하나의 deployable process로 시작한다.
- 코드 내부에서는 `domain`, `planner`, `coordinator`, `adapters` 경계를 분리해 나중의 process extraction을 가능하게 한다.
- 첫 vertical slice의 ticket, session, reservation, assignment 상태는 인메모리 repository가 소유한다.
- planner는 immutable snapshot을 입력으로 받고 side effect 없이 `ProposalBatch`를 반환한다.
- coordinator만 revision 검증, reservation, assignment mutation을 수행한다.

## Consequences

- 빠르게 executable fixture와 deterministic test를 만들 수 있다.
- serialization, RPC, distributed lock 없이 domain contract를 먼저 검증할 수 있다.
- 프로세스가 재시작되면 미확정 reservation과 in-memory state는 복구되지 않는다.
- producer는 restart 이후 active ticket과 session snapshot을 다시 제출해야 한다.
- 이 baseline은 production durability 보장이 아니라 P0/P1 검증 경로다.

## Extraction Triggers

다음 중 하나가 실제 evidence로 확인될 때 process 분리를 검토한다.

- planner와 coordinator가 서로 다른 scaling profile을 요구한다.
- 하나의 process가 reference workload의 latency 또는 throughput 목표를 충족하지 못한다.
- failure isolation이나 독립 배포가 운영상 필요하다.
- durable recovery와 다중 replica coordination이 명시적 제품 요구가 된다.

영속 저장소는 restart recovery, delivery guarantee, audit retention 요구가 구체화될 때 도입한다. 저장소 제품은 그 시점의 access pattern과 consistency contract로 선택한다.
