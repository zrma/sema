# P28 Matcher V0 Exit Gate Spec

- Status: Completed

## Objective

P0부터 P27까지 분산된 matcher correctness, quality, fairness, freshness와 bounded-search evidence를 하나의 conformance contract로 고정한다. 실제 consumer calibration이 필요한 결정을 algorithm defect와 분리하고, 기존 journal/HTTP V0 prototype을 독립적인 service productization milestone에서 재설계할 수 있게 한다.

## Acceptance

- deterministic replay, input immutability/permutation, party/capacity와 disjoint multi-proposal invariant가 fuzz/property gate에 포함된다.
- optional roster-aware backfill과 `MaxProposals` 1..4가 같은 planner fuzz를 통과한다.
- linear/indexed candidate window가 party shape, limit, empty selection과 truncation에서 exact-equivalent다.
- fuzz가 발견한 regression input을 repository corpus로 보존하고 `go test ./...`에서 항상 재실행한다.
- batch frontier, wait-priority fairness, stale roster CAS와 explicit budget evidence를 `docs/matcher-conformance.md`에서 executable test로 연결한다.
- algorithm-owned contract, consumer calibration과 service productization responsibility를 분리한다.
- full/race/performance/publication gate를 통과한다.

## Exit Decision

matcher V0의 algorithm-owned TODO는 닫는다. 이후 matcher 변경은 새 consumer evidence에 따른 policy calibration, conformance-preserving optimization 또는 명시적인 contract migration이어야 한다. persistence/API milestone의 구현 편의를 위해 matcher semantics를 암묵적으로 바꾸지 않는다.

## Completion Evidence

- planner focused fuzz가 multi-proposal/backfill 조합을 통과한다.
- discovery focused fuzz가 linear/indexed exact equality를 통과하며 no-fitting-partition의 nil/empty 차이를 잡은 corpus를 유지한다.
- exhaustive small-queue, sustained-arrival fairness, roster-aware frontier와 coordinator freshness fixture가 full gate에서 함께 통과한다.
- `docs/todo-0040-service-productization-entry/spec.md`가 다음 milestone의 input, decision gate와 첫 executable slice를 정의한다.

## Truth Boundary

이 exit는 production matchmaking quality, numeric SLA, database architecture 또는 stable API 완료를 뜻하지 않는다. point-estimate rating과 synthetic workload에서 matcher 구조가 일관된다는 증거이며 실제 MMR/traffic/region/role calibration은 consumer evidence가 소유한다.
