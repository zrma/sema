# P3 Runtime Validation Spec

- Status: Complete

## Objective

same-process engine baseline을 유지한 채 reference workload의 full application path와 process-local failure boundary를 반복 실행 가능한 evidence로 만든다. external transport나 persistence 선택 전에 실제 runtime cost와 failure surface를 확보한다.

## Deliverables

- ingestion부터 plan, reserve, confirm까지 실행하는 engine benchmark.
- representative team/battle-royale workload와 100/500/1000 ticket queue coverage.
- reservation expiry, concurrent terminal acknowledgment, restart/replay의 engine-level failure fixture.
- proposal count, unmatched reason, search budget과 lifecycle outcome을 연결하는 최소 decision-audit vocabulary.

## Acceptance

- benchmark와 fixture가 planner를 직접 호출하지 않고 `internal/engine` boundary를 통과한다.
- 같은 fixed input은 proposal와 lifecycle outcome이 반복 실행에서 같다.
- benchmark는 elapsed/allocation evidence를 생성하지만 machine-specific 수치를 제품 SLO로 고정하지 않는다.
- failure fixture는 partial reservation이나 둘 이상의 terminal outcome을 남기지 않는다.
- focused test, race detector, benchmark와 전체 repository gate가 통과한다.

## Out Of Scope

- matchmaking latency SLO와 maximum queue wait 수치 확정.
- HTTP/gRPC/queue adapter와 telemetry exporter.
- durable persistence, restart recovery와 multi-replica coordination.
- production load generator와 deployment manifest.

## Completion Evidence

- engine benchmark가 reference workload와 100/500/1000 ticket queue에서 ingestion부터 pending assignment까지 실행된다.
- benchmark가 proposal, matched/unmatched reason, search budget과 pending assignment metric을 보고한다.
- engine fixture가 reservation expiry의 whole-proposal release, concurrent terminal transition의 single winner, restart/replay boundary를 검증한다.
- focused test/race/benchmark와 full repository gate가 통과한다.

metric 정의와 측정 경계는 `docs/runtime-validation.md`가 소유한다.
