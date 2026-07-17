# Candidate Discovery

## Purpose

P7 candidate discovery boundary는 큰 queue 전체를 exact placement enumeration에 직접 넘기지 않고, policy가 명시한 deterministic ticket window만 search core에 공급한다. proposal, unmatched와 reservation contract는 바꾸지 않으며 approximation을 숨기지 않는다.

## Queue-Prefix Window

planner는 hard constraint로 ticket을 거른 뒤 enqueue time과 ticket ID로 canonical ordering한다. `internal/discovery.SelectWindow`는 현재 backfill/new-match team slot에 들어갈 수 있는 ticket 중 가장 오래된 prefix를 선택한다.

- `MaxCandidateTickets == 0`: unbounded baseline. 기존 전체 queue search behavior를 유지한다.
- 양수: proposal search 한 번에 전달할 fitting match ticket의 상한.
- 현재 slot보다 큰 party는 window quota를 소비하지 않고 건너뛴다.
- limit 뒤에 fitting ticket이 더 있으면 window는 truncated다.

초기 구현은 already-sorted queue prefix이며 skill/role/region index가 아니다. 이 package boundary는 실제 workload evidence가 생겼을 때 bucket/index 구현을 바꿀 seam이고, 현재는 oldest-demand fairness를 명시적으로 우선한다.

## Default Small-Queue Expansion

hard rejection 뒤 match ticket 12개, backfill 2개, team 2개 이하이고 `MaxCandidatesPerProposal`과 `MaxBatchCandidates`가 모두 0이면 planner는 P23 frontier와 비교 가능한 expanded path를 사용한다. 서로 다른 ticket set의 candidate를 최대 4096개까지 보존하고 feasible subset의 Pareto dominance를 먼저 검사한다. explicit candidate budget, larger queue와 single-select fast path는 기존 bounded behavior를 유지한다.

이 adaptive path는 작은 queue의 algorithmic quality gap을 줄이는 correctness 경계다. `MaxCandidateTickets`가 positive이면 그 window 밖 demand를 다시 포함하지 않으며, 4096 candidate와 global node budget을 production capacity나 full enumeration 보장으로 해석하지 않는다.

## Discovery And Batch Budgets

| Policy field | Bounded unit | Purpose |
|---|---|---|
| `MaxCandidateTickets` | enumeration에 들어가는 match ticket | queue partition/window |
| `MaxCandidatesPerProposal` | exact-capacity placement evaluation 수 | quality comparison |
| `MaxSearchNodes` | recursive search node | candidate generation 전체 computation |
| `MaxBatchCandidates` | deduplicated proposal candidate | global selection graph |
| `MaxBatchSearchNodes` | branch-and-bound node | global batch selection |

다섯 값은 canonical policy fingerprint에 모두 포함된다. 같은 policy version에서 budget 하나만 바꿔도 fingerprint와 proposal identity가 달라진다.

## Evidence And Failure Semantics

proposal `ScoreEvidence`는 다음 discovery evidence를 추가한다.

- `CandidateTickets`: 해당 search window에 들어간 ticket 수.
- `CandidateWindowTruncated`: fitting supply가 window limit 뒤에도 있었는지 여부.
- `SearchTruncated`: candidate ticket, exact candidate 또는 node budget 중 하나라도 잘렸는지 여부.

window 또는 candidate-generation truncation과 global-selection truncation은 batch `BudgetExhausted`를 true로 만든다. window 안에서 match를 찾지 못했고 fitting supply가 더 있었다면 unmatched 대표 reason은 `search_budget`이다. proposal limit에 도달한 뒤 남은 demand는 `proposal_limit`, admissible 대안이 있었지만 더 높은 total batch utility에 포함되지 않은 경우는 `batch_objective`다.

## Quality And Fairness Tradeoff

oldest prefix는 queue fairness를 예측 가능하게 만들지만 뒤쪽의 더 좋은 skill/role 조합을 놓칠 수 있다. `diagnostic-candidate-window-gap`은 oldest two tickets의 skill gap 1000을 선택하고 exhaustive oracle이 뒤쪽 gap 0을 찾아 `oracle_preferred`를 기록한다.

따라서 window는 성능을 위한 opt-in approximation이며 quality 개선으로 해석하지 않는다. 실제 policy 값은 queue size, wait distribution과 allowed quality gap을 함께 측정해 정한다.

## Wait-Priority Service

P25부터 각 match ticket과 backfill ticket은 자신의 enqueue time으로 active relaxation step을 계산한다. `PrioritizeWait`에 들어간 demand가 admissible candidate에 포함되면 selector는 backfill-count tier 안에서 다음 순서를 rank utility보다 먼저 적용한다.

1. selected batch에 포함된 가장 오래된 priority demand의 wait.
2. 그 oldest wait가 같을 때 selected priority demand 수.
3. 기존 small-queue Pareto 또는 bounded rank utility와 canonical tie-break.

oldest fitting prefix는 오래된 demand를 window 앞에 유지하고 anchored generation은 그 demand를 포함하는 admissible candidate를 찾는다. 다만 explicit window/candidate/node budget으로 candidate가 생성되지 않으면 service invariant의 전제가 성립하지 않으며 `BudgetExhausted`와 truncation evidence가 이를 표시한다. hard-rejected demand는 eligible count에 포함되지 않고, admissible candidate에는 있었지만 이번 batch에서 선택되지 않은 demand는 eligible-selected 차이로 관찰할 수 있다.

## Capacity Evidence

- 10K solo queue는 normal test에서 256-ticket window, exact capacity, unmatched materialization과 evidence를 검증한다.
- 10K/100K unbounded와 window-256 비교는 planner benchmark gate에서 각 1회 실행한다.
- 현재 `ProposalBatch`가 모든 unmatched ticket을 반환하므로 100K cost의 대부분은 전체 queue ordering/copy와 unmatched materialization에 남는다.
- full unmatched contract를 summary/cursor로 바꾸는 것은 public API milestone의 별도 contract decision이다.

```sh
go test ./internal/planner -run '^TestPlanTenThousandTicketCandidateWindow$'
go test ./internal/planner -run '^$' -bench '^BenchmarkPlanLargeQueues$' -benchtime=1x -count=1
go test ./internal/planner -run '^$' -fuzz '^FuzzPlanInvariants$' -fuzztime=10s
```

elapsed time과 allocation은 target hardware SLO가 아니며 P10 calibration 전에는 관찰 evidence로만 사용한다.

## Reusable Partition Index

P27의 `internal/discovery.Index`는 canonical queue를 다음 key로 partition한다.

- party size.
- party average skill의 100-point band.
- role multiset의 stable numeric profile.
- party maximum latency의 25ms band.

partition은 candidate preference를 바꾸지 않는다. typical party-size envelope은 `partySize <= max slot`인 canonical position list를 미리 만들고, size 종류가 16개를 넘는 경우 partition head heap merge로 fallback한다. 두 경로 모두 `SelectWindow`의 oldest fitting prefix와 `Truncated`를 byte-for-value equivalent하게 반환한다. role profile collision은 partition을 합칠 수만 있고 canonical merge 결과는 바꾸지 않는다.

96-ticket shape/limit matrix와 10K mixed-party queue가 linear/indexed `Window` 전체를 `DeepEqual`로 비교한다. 100K queue의 four-shape reuse benchmark는 indexed lookup이 linear scan과 같은 microsecond-scale임을 확인하지만 shape와 단일 실행 noise에 따라 어느 쪽도 더 빠를 수 있고, index build cost는 lookup 차이보다 훨씬 크다. 따라서 current stateless planner는 즉석 rebuild를 하지 않는다. queue mutation에 따라 index를 incremental하게 유지하고 여러 plan에 재사용하는 ownership은 P28 이후 persistence/API productization entry에서 stateful demand repository와 함께 연결한다. 이 결정은 algorithm gap이 아니라 lifecycle placement다.

```sh
go test ./internal/discovery -run 'TestIndexedWindowMatchesLinear'
go test ./internal/discovery -run '^$' -bench 'Benchmark(BuildIndex|WindowSelectionReuse)' -benchmem
```
