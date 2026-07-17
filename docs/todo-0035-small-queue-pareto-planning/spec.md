# P24 Small-Queue Pareto Planning Spec

- Status: Completed

## Objective

P23 frontier가 default budget의 작은 mixed-party workload에서 찾은 global batch quality gap을 닫는다. individual best proposal의 rank 합만으로 batch를 고르면 coverage와 wait가 같은데도 모든 per-match quality dimension이 더 나쁜 조합을 선택할 수 있다. 작은 queue에서는 더 넓은 candidate graph와 Pareto dominance를 사용하고, 큰 queue와 explicit approximation policy의 bounded behavior는 보존한다.

## Adaptive Boundary

다음 조건을 모두 만족할 때만 small-queue expanded search를 사용한다.

- policy의 `MaxCandidatesPerProposal`과 `MaxBatchCandidates`가 모두 0이다.
- hard rejection 뒤 available match ticket이 최대 12개다.
- backfill ticket이 최대 2개이고 team이 최대 2개다.

per-placement와 batch candidate limit은 각각 4096으로 확장하지만 기존 global generation/selection node budget은 유지한다. explicit candidate budget이 있거나 범위를 넘는 입력은 P18/P20 bounded path를 그대로 사용한다.

## Candidate And Selection Contract

- expanded placement search는 같은 team placement만 반복하지 않고 서로 다른 ticket set의 best placement를 보존한다.
- global selector는 feasible candidate subset만 열거하고 ticket/backfill conflict가 생기면 즉시 건너뛴다.
- small-queue selection ordering은 backfill 수, proposal 수, matched player 수를 먼저 보존한다.
- 같은 coverage tier에서는 oldest/mean wait를 maximize하고 maximum/mean role penalty, team skill gap과 maximum latency를 minimize하는 Pareto dominance를 먼저 적용한다.
- 서로 지배하지 않는 trade-off는 기존 total rank utility와 canonical key로 결정한다.
- explicit budget path의 rank utility, truncation과 unmatched reason semantics는 바꾸지 않는다.

## Acceptance

- weighted party/skill/role/latency/wait와 optional backfill을 가진 seed 1..128 default small workload가 모두 exhaustive frontier와 `frontier_equivalent`다.
- differential corpus에서 candidate generation과 selection은 truncation되지 않는다.
- higher rank-sum batch가 maximum/mean skill gap에서 모두 나쁘면 lower rank-sum nondominated batch를 선택한다.
- explicit one-candidate diagnostic은 계속 `frontier_dominated`와 budget exhaustion을 노출한다.
- single-select, 50v50, battle royale, 100K queue와 engine benchmark가 기존 fast path allocation 수준을 유지한다.
- public Go alpha marker가 `v0alpha3`로 바뀌고 zero-value candidate budget의 의미 변경과 rollback 제한이 migration 문서에 남는다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- 이 경로는 P23과 같은 small synthetic boundary의 correctness improvement다. production queue 전체를 exhaustive하게 탐색한다는 뜻이 아니다.
- Pareto ordering은 calibrated product utility가 아니며 incomparable trade-off의 제품 선호는 기존 rank utility에 남는다.
- 4096 candidate limit과 100000 selection node 기본값은 safety bound이지 traffic SLO가 아니다.

## Completion Evidence

- 첫 differential failure는 2 proposal/12 player coverage와 wait가 같지만 planner maximum skill gap 85, exhaustive witness 17이었다.
- expanded candidate graph와 Pareto subset search 뒤 128개 deterministic workload가 모두 `frontier_equivalent`이며 generation/selection truncation이 0이다.
- direct selector fixture는 utility 200의 gap 0/100 pair 대신 utility 160의 gap 10/10 nondominated pair를 선택한다.
- max-one-candidate P23 diagnostic과 explicit bounded behavior는 그대로 유지된다.
- large/single-select benchmark에서 alternative collection을 생략해 기존 reference allocation을 유지한다.
- `docs/migrations/v0alpha2-to-v0alpha3.md`가 public objective-ordering 변경을 소유하며 HTTP service `v0alpha1`과 독립된 marker임을 명시한다.
