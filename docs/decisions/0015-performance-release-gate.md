# ADR 0015: Reference SLO And Version-Aware Release Admission

- Status: Accepted

## Context

machine-specific one-shot benchmark는 release regression을 판정하기 어렵고 raw CI output은 host/CPU 정보를 포함한다. 실제 production consumer와 hardware는 아직 없지만 P10을 닫으려면 service recovery, latency와 allocation의 반복 가능한 수치 gate가 필요하다. 동시에 alpha surface를 가진 상태에서 v1 tag가 같은 generic release workflow를 통과하면 stability를 잘못 선언할 수 있다.

## Decision

- pinned Linux container, 2 CPU/2 GiB와 local volume을 reference target profile v1으로 정의한다.
- service lifecycle/recovery를 3회 반복하고 p95 250ms, max request 1s와 10-cycle duration 30s를 적용한다.
- representative planner/engine/replay benchmark를 최소 3회 반복하고 worst ns/op, B/op와 allocs/op budget을 적용한다.
- raw benchmark output은 폐기하고 aggregate profile/report만 CI artifact로 30일 보존한다.
- version-aware admission은 v0 alpha에 full/container/performance/recovery/publication gate를 요구한다.
- major version 1 이상은 machine-readable stable admission flag가 true가 아니면 gate 시작 전에 차단한다.
- current stable flag는 false이며 stable API, authenticated transport, external consumer와 support/retention evidence가 blocker다.

## Consequences

- order-of-magnitude performance/allocation regression과 recovery failure를 release 전에 잡는다.
- runner 차이를 product SLA로 오해하지 않는 bounded numeric baseline을 얻는다.
- P10 completion은 v1 readiness 주장이 아니라 executable stable-release protection을 의미한다.
- performance job은 image build와 repeated benchmark 때문에 일반 unit gate보다 오래 걸린다.

## Revisit Triggers

- actual production hardware/workload가 reference profile을 대체한다.
- benchmark variance가 current headroom 안에서 반복 false positive를 만든다.
- journal compaction/database, authenticated transport 또는 multi-replica가 profile을 바꾼다.
- stable admission blocker가 모두 evidence와 함께 해결된다.
