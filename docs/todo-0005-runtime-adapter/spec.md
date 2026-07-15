# P3 Runtime Adapter Decision Spec

- Status: Complete

## Objective

`internal/engine`을 사용하는 첫 integration boundary와 그 경계가 요구하는 delivery/recovery contract를 현재 consumer topology에 맞춰 선택한다.

## Available Baseline

- side-effect-free deterministic planner.
- in-process single-writer coordinator와 fixed-TTL reservation.
- terminal assignment acknowledgment와 backfill roster CAS evidence.
- transport-neutral engine facade.
- process restart recovery를 제공하지 않는 명시적 in-memory boundary.

## Decision Input

1. 첫 consumer는 같은 Go process에서 `internal/engine`을 직접 호출한다.
2. producer는 process restart 뒤 active match/backfill demand 전체를 다시 제출한다.
3. assignment consumer는 같은 call path에서 synchronous acknowledgment를 반환한다.
4. 첫 deployment는 single replica이며 coordinator authority를 process 안에 유지한다.
5. remote module identity와 public API compatibility는 아직 고정하지 않는다.

## Decision

`internal/engine.Engine`을 첫 same-process Go adapter로 사용한다. 별도의 forwarding wrapper, HTTP/gRPC endpoint, queue consumer, database를 추가하지 않는다. caller가 policy를 명시적으로 등록하고 time, snapshot/reservation/assignment/operation ID를 제공하며 engine은 typed domain result를 그대로 반환한다.

delivery는 direct call completion을 기준으로 한다. 같은 payload와 idempotency ID의 retry는 process lifetime 안에서 동일 결과를 반환하며 conflicting reuse는 `IdempotencyConflict`다. transport timeout은 없고 caller context나 network retry contract도 아직 public surface가 아니다.

restart는 recovery가 아니라 새 empty engine 시작이다. active demand의 source of truth인 producer가 전체 snapshot을 재제출해야 하며 active reservation과 assignment read model은 복구하지 않는다. durable delivery나 multi-replica 요구가 생기면 adapter와 persistence를 함께 다시 결정한다.

## Selection Consequences

- same-process Go: 가장 작은 integration이지만 process failure domain과 state를 공유한다.
- synchronous RPC: 명확한 request/response와 typed failure mapping이 필요하고 client retry/idempotency 계약을 노출한다.
- asynchronous queue: delivery duplication, ordering, outbox/inbox, delayed acknowledgment를 처음부터 durable하게 설계해야 한다.
- multi-replica: reservation authority와 assignment source of truth를 외부 durable CAS store로 옮겨야 한다.

## Acceptance

- direct engine call의 request, response, idempotency, retry, timeout 경계를 이 문서에 고정한다.
- 별도 mapping layer 없이 domain failure와 unmatched reason을 그대로 보존한다.
- end-to-end lifecycle과 restart/replay scenario를 engine fixture로 실행한다.
- public protocol, durable repository, migration/recovery contract를 추가하지 않는다.

## Completion Evidence

- `internal/engine` fixture가 repeated plan, reserve/confirm retry와 synchronous acknowledgment를 실행한다.
- 새 engine은 이전 process-local state를 보지 못하며 producer replay 뒤 같은 snapshot에서 동일 proposal을 만든다.
- 동일 reservation ID는 fresh process에서 다시 사용할 수 있어 idempotency scope가 process-local임을 드러낸다.
- focused engine test와 full/race repository gate가 통과한다.

## Out Of Scope

- 임의의 HTTP/gRPC/queue 선택.
- database 제품 선택.
- multi-replica lock 또는 leader election.
- public SDK와 compatibility guarantee.

## Durable Decision

장기 architecture consequences와 extraction trigger는 `docs/decisions/0002-runtime-adapter-baseline.md`가 소유한다.
