# Workload Evaluation

## Purpose

P6 evaluation baseline은 production traffic을 추정하지 않고도 planner 변경을 같은 입력과 quality vocabulary로 비교하기 위한 deterministic evidence layer다. seeded synthetic queue, player-level coverage/fairness metric과 small-case exhaustive oracle을 `sema-lab`에 연결한다.

이 baseline은 특정 게임의 실제 party/skill/latency 분포나 numeric SLO를 주장하지 않는다. 실제 telemetry 또는 consumer input이 생기면 model parameter와 regression budget을 별도 decision으로 calibration한다.

## Synthetic Snapshot Model

`internal/evaluation.WorkloadModel`은 다음 queue snapshot 축을 고정한다.

- stable scenario ID, fixed `now`와 explicit seed.
- ticket count와 weighted party-size distribution.
- point-estimate skill center와 uniform spread.
- weighted role distribution.
- latency와 queue-wait range.

fixture generator는 repository-owned SplitMix64 implementation을 사용하므로 standard-library random implementation 변화와 무관하다. 같은 validated model은 entity identity, party composition, skill, role, latency와 enqueue time이 같은 immutable scenario를 만든다.

현재 model의 `Skill`은 point estimate다. rating deviation 또는 confidence를 domain contract에 추가하거나 objective에 반영하는 방식은 실제 rating model 없이 확정하지 않는다.

이 calibration 결정과 deterministic regression budget은 ADR 0007 및 `docs/evaluation-baseline.md`가 소유한다.

## Metric Vocabulary

| Metric | Meaning |
|---|---|
| `coverage_basis_points` | matched players / demand players를 0..10000 정수로 표현한 값 |
| `matched_tickets`, `matched_players` | proposal에 포함된 party ticket 수와 player 수; 서로 대체하지 않음 |
| `unmatched_tickets`, `unmatched_players` | cycle 뒤 남은 demand의 ticket/player 수 |
| `oldest_matched_wait_millis` | 선택된 demand 중 가장 긴 snapshot wait |
| `oldest_unmatched_wait_millis` | 남은 demand 중 가장 긴 snapshot wait; queue starvation 관찰용 |
| `total_role_penalty` | proposal evidence의 soft role deficit 합계 |
| `max_team_skill_gap` | proposal 중 가장 큰 team average skill gap |
| `max_latency_millis` | matched player의 최대 latency evidence |
| `search_nodes`, `budget_exhausted` | bounded search cost와 truncation outcome |
| `wait_priority_eligible/selected_demands` | candidate graph에서 age service 대상이 된 demand와 이번 batch에서 service된 수 |

coverage와 wait는 queue 결과이고 role/skill/latency는 선택된 proposal quality다. 하나의 scalar score로 합치지 않고 objective ordering과 함께 해석한다.

## Exhaustive Oracle

oracle은 최대 두 team과 12개의 match ticket을 가진 new-match snapshot에서 party를 쪼개지 않고 모든 exact-capacity team placement를 열거한다. hard constraint와 현재 time-dependent objective를 그대로 평가해 admissible candidate 중 lexicographic best quality vector를 찾는다.

`sema-lab`은 eligible workload에서 planner 첫 proposal과 oracle을 다음 relation으로 비교한다.

- `equivalent`: 현재 objective 비교에서 같은 quality vector다.
- `oracle_preferred`: bounded planner가 더 좋은 admissible vector를 놓쳤다.
- `planner_preferred`: exhaustive result와 모순되는 상태이며 회귀 조사 대상이다.

oracle은 proposal ID, canonical tie-break, 여러 proposal을 합친 global batch optimum 또는 backfill optimum을 증명하지 않는다. 작은 single-proposal quality gap을 측정하는 correctness/evaluation 도구다.

## Batch Quality Frontier

P23 frontier는 기존 single-proposal oracle과 별개로 최대 12 match ticket, 최대 2 backfill ticket, 최대 2 team인 snapshot의 exact-capacity candidate와 disjoint batch를 모두 열거한다. 각 candidate는 기존 hard constraint와 time-dependent admissibility threshold를 통과해야 하며 batch는 `MaxProposals`, ticket uniqueness와 backfill-target uniqueness를 지킨다.

frontier는 하나의 임의 scalar를 만들지 않고 다음 Pareto dimension을 사용한다.

- maximize: selected backfill, proposal count, matched players, oldest/mean matched wait.
- minimize: maximum/mean role penalty, maximum/mean team skill gap, maximum latency.
- evidence only: matched ticket count와 role/skill/wait total.

planner batch는 exhaustive nondominated point와 같은 dimension이면 `frontier_equivalent`, 모든 dimension에서 나쁘지 않고 하나 이상 더 좋은 witness가 있으면 `frontier_dominated`, trade-off만 있으면 `frontier_incomparable`다. `SelectionUtility`는 bounded candidate graph 안의 상대 rank이므로 comparison dimension이 아니다. 이 frontier는 production planner 대체 경로가 아니라 candidate coverage와 batch-quality approximation gap을 드러내는 작은 입력 평가 도구다.

P24부터 candidate budget을 명시하지 않은 같은 small boundary는 expanded candidate generation과 Pareto subset selection을 사용한다. weighted party/skill/role/latency/wait 및 optional backfill의 seed 1..128 differential corpus는 planner batch가 exhaustive frontier와 모두 `frontier_equivalent`이고 truncation이 없는지 검증한다. explicit low budget diagnostic은 approximation evidence를 유지하므로 이 gate의 대상이 아니다.

P25 sustained-arrival diagnostic은 quality-first 구간에 오래된 불균형 pair를 남겨 둔 채 매 10초마다 새로운 균형 pair를 추가한다. 30초 `PrioritizeWait` 경계 전에는 fresh quality를 선택하지만, 경계에 도달한 첫 cycle에는 oldest pair와 evidence의 eligible/selected 2개를 반드시 service한다. 이 30초는 fixture policy 값이지 production wait SLA가 아니다.

P26 roster-aware backfill fixture는 기존 team skill total 1000/1500과 healer/dps role count에 두 incoming player를 배치한다. planner와 exhaustive frontier는 high-dps를 낮은 team에, low-healer를 높은 team에 넣어 resulting skill gap 0, role penalty 0과 max latency 60을 만드는 point에서 equivalent여야 한다. context가 없는 기존 frontier corpus는 vacancy-only baseline을 계속 검증한다.

## Reference Evidence

- `synthetic-5v5-seeded-queue`: weighted party/skill/role/latency/wait distribution의 multi-proposal coverage와 queue evidence.
- `diagnostic-bounded-quality-gap`: candidate limit 1이 첫 feasible 1:1 proposal을 선택하고 oracle이 더 낮은 skill gap을 찾는 의도적 approximation fixture.
- `batch-frontier-mixed-party-backfill`: solo/duo/trio, backfill과 new match를 한 batch에서 모두 선택하는 equivalent fixture.
- `diagnostic-batch-frontier-gap`: candidate graph를 한 proposal로 제한해 exhaustive 2-proposal witness가 planner를 지배하는 fixture.
- team workload 중 oracle ticket bound 안에 드는 case: planner와 exhaustive quality relation 확인.

```sh
go run ./cmd/sema-lab synthetic-5v5-seeded-queue
go run ./cmd/sema-lab -format json diagnostic-bounded-quality-gap
go run ./cmd/sema-lab -format json batch-frontier-mixed-party-backfill diagnostic-batch-frontier-gap
```

JSON envelope은 P23 batch frontier가 추가된 `v0alpha5`이며 stable compatibility를 아직 약속하지 않는다. 기존 exhaustive oracle은 single-proposal quality를 계속 비교하고 batch frontier는 eligible small snapshot의 global disjoint batch를 비교한다.

## Verification

- focused: `go test ./internal/evaluation ./internal/lab ./cmd/sema-lab`.
- race: `go test -race ./internal/evaluation ./internal/lab ./cmd/sema-lab`.
- full repository gate: `scripts/check.sh`.
