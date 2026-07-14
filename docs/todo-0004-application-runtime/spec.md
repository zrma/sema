# P3 Application Runtime Spec

- Status: Complete

## Objective

domain, planner, coordinator package를 하나의 transport-neutral application service로 조합한다. 이후 HTTP/gRPC/queue adapter가 matchmaking semantics를 재구현하지 않고 동일 use case를 호출하게 한다.

## Deliverables

- `internal/engine`의 constructor와 ticket/backfill ingestion facade.
- snapshot/plan, reserve, confirm, reservation cancel, assignment acknowledgment/read facade.
- 모든 시간과 idempotency ID를 caller가 명시하는 deterministic interface.
- submit부터 completed assignment까지의 new-match end-to-end fixture.
- backfill stale acknowledgment와 cancellation end-to-end fixture.

## Acceptance

- adapter는 `planner`와 `coordinator`를 직접 조합하지 않고 engine 한 경계로 전체 lifecycle을 실행할 수 있다.
- engine은 domain failure code와 immutable/defensive result를 그대로 보존한다.
- 같은 fixture를 반복해 ordered proposal과 terminal assignment 결과가 같다.
- new-match와 backfill flow가 full/race gate에서 끝까지 실행된다.
- engine은 wall clock, ID generator, network, storage implementation을 숨겨서 만들지 않는다.

## Decision Gate

P3 facade 이후 외부 transport와 durable persistence는 실제 consumer, delivery guarantee, deployment topology가 필요하다. 그 정보 없이 HTTP/gRPC, queue, database 제품을 암묵적으로 선택하지 않는다.

## Out Of Scope

- public API compatibility.
- HTTP/gRPC protocol과 authentication.
- executable server process와 deployment manifest.
- durable repository와 process restart recovery.
- metrics backend와 distributed tracing exporter.

## Completion Evidence

- `internal/engine`이 ingestion, snapshot/plan, reserve/confirm/cancel, assignment acknowledgment/read를 한 경계로 제공한다.
- facade는 wall clock과 ID를 생성하지 않고 caller 입력을 그대로 사용한다.
- new-match flow가 repeated deterministic plan부터 completed assignment까지 실행된다.
- backfill stale roster failure가 terminal assignment read model까지 실행된다.
- active reservation이 새 cycle에서 제외되고 cancel 뒤 다시 planning되는 흐름을 실행한다.
- engine end-to-end fixture를 포함한 full/race gate가 통과한다.
