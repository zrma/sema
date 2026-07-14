# P3 Runtime Adapter Decision Spec

- Status: Decision Required

## Objective

`internal/engine`을 사용하는 첫 external adapter와 그 adapter가 요구하는 delivery/recovery contract를 실제 consumer topology에 맞춰 선택한다.

## Available Baseline

- side-effect-free deterministic planner.
- in-process single-writer coordinator와 fixed-TTL reservation.
- terminal assignment acknowledgment와 backfill roster CAS evidence.
- transport-neutral engine facade.
- process restart recovery를 제공하지 않는 명시적 in-memory boundary.

## Required Decision Input

1. 첫 consumer가 같은 Go process, request/response service, asynchronous worker 중 무엇인가.
2. ticket과 backfill producer가 snapshot 전체를 재제출할 수 있는가, event delivery를 복구해야 하는가.
3. assignment consumer가 synchronous acknowledgment를 반환하는가, retry 가능한 message를 보내는가.
4. 첫 deployment가 single replica인지 multi-replica coordination을 바로 요구하는가.
5. remote module/repository identity와 public API compatibility를 지금 고정할 필요가 있는가.

## Recommended Default

실제 consumer가 아직 없다면 protocol과 database를 추가하지 않는다. 첫 integration이 같은 backend 안에서 시작할 수 있으면 in-process Go adapter로 domain semantics와 load를 먼저 검증하고, process 분리 또는 durable delivery 요구가 확인될 때 RPC/queue와 persistence를 함께 선택한다.

## Selection Consequences

- same-process Go: 가장 작은 integration이지만 process failure domain과 state를 공유한다.
- synchronous RPC: 명확한 request/response와 typed failure mapping이 필요하고 client retry/idempotency 계약을 노출한다.
- asynchronous queue: delivery duplication, ordering, outbox/inbox, delayed acknowledgment를 처음부터 durable하게 설계해야 한다.
- multi-replica: reservation authority와 assignment source of truth를 외부 durable CAS store로 옮겨야 한다.

## Acceptance After Decision

- 선택한 adapter의 request, response, idempotency, retry, timeout contract를 fixture로 고정한다.
- domain failure와 unmatched reason을 손실 없이 mapping한다.
- end-to-end lifecycle과 process/retry failure scenario를 실행한다.
- 필요할 때만 durable repository와 migration/recovery contract를 함께 추가한다.

## Out Of Scope Until Input Exists

- 임의의 HTTP/gRPC/queue 선택.
- database 제품 선택.
- multi-replica lock 또는 leader election.
- public SDK와 compatibility guarantee.
