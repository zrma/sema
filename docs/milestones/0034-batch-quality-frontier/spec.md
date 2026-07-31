# P23 Batch Quality Frontier Spec

- Status: Completed

## Objective

P18 selector가 bounded candidate graph 안에서 선택한 batch를 작은 snapshot의 전체 feasible batch 공간과 비교한다. 실제 traffic target 없이 임의의 calibrated scalar를 만들지 않고, coverage와 per-match quality가 함께 개선될 수 있는 지점을 deterministic Pareto frontier로 측정한다.

## Exhaustive Boundary

- 최대 12개의 `MatchTicket`, 최대 2개의 `BackfillTicket`과 최대 2개 team인 validated snapshot만 exhaustive frontier 대상이다.
- new-match와 backfill의 exact-capacity placement를 모두 열거하고 hard constraint와 현재 time-dependent quality threshold를 통과한 candidate만 batch 조합에 사용한다.
- batch는 policy의 `MaxProposals` 이하이며 match ticket과 backfill target을 중복 사용하지 않는다.
- ticket, backfill과 candidate 입력 순서가 달라도 같은 frontier와 comparison을 반환한다.
- 이 범위를 넘는 snapshot은 frontier를 생략하며 bounded planner의 production 대체 경로로 사용하지 않는다.

## Frontier Contract

frontier point는 다음 aggregate를 기록한다.

- maximize: selected backfill, proposal count, matched players, oldest/mean matched wait.
- minimize: maximum/mean role penalty, maximum/mean team skill gap, maximum latency.
- observe: matched ticket count와 role/skill/wait total.

한 point가 모든 maximize dimension에서 작지 않고 모든 minimize dimension에서 크지 않으며 적어도 한 dimension에서 더 좋으면 다른 point를 지배한다. planner point는 다음 relation 중 하나를 가진다.

- `frontier_equivalent`: 같은 aggregate의 nondominated exhaustive point가 있다.
- `frontier_dominated`: 모든 dimension에서 나쁘지 않고 하나 이상 더 좋은 exhaustive point가 있다.
- `frontier_incomparable`: exhaustive frontier와 trade-off 관계지만 같은 point는 아니다.

frontier 순서는 comparison 의미가 아니라 deterministic report ordering만 제공한다. `SelectionUtility`는 candidate graph 내부 상대 rank이므로 frontier dimension으로 사용하지 않는다.

## Report Contract

- `sema-lab` experimental JSON envelope을 `v0alpha5`로 올리고 eligible workload에 planner point, relation, dominating witness, exhaustive candidate/batch count와 전체 frontier point를 기록한다.
- 기본 text report는 relation, search size, planner point와 dominating witness를 요약한다.
- solo/duo/trio party와 backfill을 함께 포함하는 small mixed workload를 corpus에 추가한다.
- 의도적으로 `MaxBatchCandidates`를 제한한 diagnostic은 planner batch가 exhaustive frontier에 지배되는 evidence를 만든다.

## Acceptance

- 충분한 budget의 mixed new-match/backfill workload는 `frontier_equivalent`다.
- bounded candidate diagnostic은 더 많은 player와 proposal을 품질 저하 없이 포함하는 frontier point에 `frontier_dominated`다.
- input permutation은 candidate count, batch count, frontier와 relation을 바꾸지 않는다.
- overlapping ticket 또는 backfill target을 사용하는 batch는 comparison에서 거부한다.
- legacy single-proposal oracle과 existing report field는 유지한다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- frontier는 작은 synthetic snapshot의 exhaustive evidence이며 production traffic, MMR confidence, SLA 또는 calibrated utility가 아니다.
- 각 proposal은 기존 relaxation threshold를 이미 통과한다. frontier는 threshold 자체의 적절성을 증명하지 않는다.
- matched player와 wait를 늘리면서 quality aggregate가 나빠지는 point는 trade-off이지 자동 권장안이 아니다.

## Completion Evidence

- mixed solo/duo/trio + backfill workload는 5 ticket/11 player를 backfill 1개와 new match 1개로 모두 선택하고 `frontier_equivalent`를 반환한다.
- `MaxBatchCandidates=1` diagnostic은 planner의 1 proposal/2 player point를 2 proposal/4 player exhaustive witness가 지배해 `frontier_dominated`를 반환한다.
- mixed fixture의 exhaustive counters는 5 placements, 2 admissible candidates, 4 batches이며 diagnostic은 12 placements, 6 admissible candidates, 10 batches다.
- input permutation, ticket/backfill conflict와 12-ticket/2-backfill/2-team safety bound를 focused test로 고정했다.
- `sema-lab` text/JSON report가 `v0alpha5` frontier summary와 full point evidence를 출력한다.
