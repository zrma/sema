# P10 Operational Validation Spec

- Status: Complete

## Objective

실제 service lifecycle 부하와 durable restart/failure recovery를 외부 상태를 변경하지 않는 하나의 반복 가능한 command로 검증한다.

## Deliverables

- bounded concurrent HTTP lifecycle workload runner.
- aggregate latency/count/metrics JSON report.
- completed assignment와 audit의 restart replay check.
- incomplete journal tail failure injection과 recovery check.
- quick repository gate와 manual soak procedure.

## Acceptance

- 매 cycle의 모든 ticket이 disjoint proposal과 terminal assignment에 포함된다.
- multi-cycle 실행이 duplicate ownership이나 stale demand 없이 완료된다.
- metrics route counter와 redacted audit 전체를 읽을 수 있다.
- restart 뒤 completed assignment와 audit prefix가 보존된다.
- incomplete final record가 complete prefix 손상 없이 제거된다.
- report가 resource ID와 local path를 노출하지 않는다.
- focused race test와 full repository/publication gate가 통과한다.

## Out Of Scope

- external production/staging endpoint 부하.
- sudden kill, filesystem full와 device power-loss 보장.
- target hardware numeric SLO와 alert budget.
- container, orchestration, authentication과 multi-replica deployment.

## Completion Evidence

`go test -race ./internal/operational ./cmd/sema-ops-check`와 quick `go run ./cmd/sema-ops-check`가 통과한다. workload/recovery contract는 `docs/operational-validation.md`, architecture decision은 ADR 0013이 소유한다.
