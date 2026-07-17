# P20 Single-Select Performance Spec

- Status: Completed

## Objective

P18 global candidate graph가 selector에서 둘 이상의 proposal을 선택할 수 없는 workload에도 anchored alternative를 생성해 P10 reference performance budget을 초과한 회귀를 제거한다. multi-proposal batch semantics와 budget은 유지하면서 set-packing 충돌 대안이 의미 없는 경우에는 기존 unanchored best search 하나만 실행한다.

## Contract

- `MaxCandidatesPerProposal`이 기본 64 이상이고 `MaxProposals == 1`이면 new-match와 각 backfill shape는 unanchored best candidate만 생성한다.
- per-search candidate budget이 기본 64 이상이고 backfill이 없으며 available player 수로 최대 한 new match만 채울 수 있으면 anchored new-match alternative를 생성하지 않는다.
- 작은 per-search candidate budget에서는 anchor가 queue-order 편향을 완화하므로 single-select라도 anchored alternative를 유지한다.
- 둘 이상의 proposal을 선택할 수 있거나 backfill과 new-match가 ticket을 두고 경쟁할 수 있으면 greedy cover와 anchored candidate graph를 그대로 생성한다.
- fast path는 proposal validation, admissibility, objective ordering, evidence, deterministic identity와 selector를 우회하지 않는다.
- reference performance budget을 높여 회귀를 숨기지 않는다.

## Acceptance

- single-proposal limit fixture가 candidate 하나와 proposal 하나를 반환한다.
- 50v50 exact-capacity fixture가 candidate generation node 1,000 미만으로 proposal 하나를 반환한다.
- one-candidate search budget의 quality-gap fixture가 anchored fallback과 기존 planner quality vector를 보존한다.
- 기존 multi-proposal fixture는 selected proposal 수보다 큰 diverse candidate graph를 계속 관찰한다.
- planner 50v50, planner 100K/window-256와 engine 1,000-ticket benchmark가 기존 `sema-reference-container-v1` budget을 통과한다.
- focused, full, race, container performance와 publication boundary gate를 통과한다.

## Truth Boundary

- fast path는 selector cardinality가 하나로 제한된 경우의 중복 candidate generation만 제거한다. multi-proposal search complexity나 32-match Flow burst의 production capacity를 증명하지 않는다.
- benchmark 수치는 reference container regression evidence이며 production latency SLO가 아니다.

## Completion Evidence

- local focused benchmark에서 planner 50v50은 약 0.39ms, 0.73MB와 1.27K allocations, planner 100K/window-256은 약 33ms, 53.4MB와 101K allocations를 기록했다.
- engine 1,000-ticket lifecycle은 약 1.0ms, 1.70MB와 3.5K allocations를 기록했다.
- 세 항목은 P20 fast path 적용 전의 P18 implementation에서 각각 약 44ms/63.9MB/79.6K, 54ms/58.7MB/167K와 8.9ms/7.46MB/69K allocations였다.
- 최종 2 CPU/2 GiB Docker reference profile, 전체 repository/race/release-build gate와 기존 one-candidate quality-gap fixture가 함께 통과했다.
