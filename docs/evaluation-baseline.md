# Evaluation Regression Baseline

## Scope

이 baseline은 machine timing이 아니라 같은 policy/workload에서 재현되는 player coverage, search-node ceiling과 oracle relation을 회귀 gate로 사용한다. 목적은 planner optimization이 탐색 비용을 늘리거나 match quality를 낮출 때 elapsed-time noise 없이 먼저 감지하는 것이다.

## Baseline V1

| Workload | Coverage gate | Search gate | Quality gate |
|---|---:|---:|---|
| team/battle-royale reference corpus | exactly 10000 bp | configured bounded search 안 | all demand matched |
| `synthetic-5v5-seeded-queue` | at least 6000 bp | at most 1000 nodes | stable unmatched wait와 score evidence |
| `diagnostic-bounded-quality-gap` | exactly 5000 bp | selected search at most 5 nodes, generation at most 30 nodes | `oracle_preferred`, planner gap 500, oracle gap 0 |
| `diagnostic-candidate-window-gap` | exactly 5000 bp | 2-ticket truncated window | `oracle_preferred`, planner gap 1000, oracle gap 0 |
| `batch-frontier-mixed-party-backfill` | exactly 10000 bp | 5 placements, 2 admissible candidates, 4 batches | `frontier_equivalent`, 1 backfill, 2 proposals, 11 players |
| `diagnostic-batch-frontier-gap` | exactly 5000 bp | generation at most 30 nodes; 12 placements, 6 candidates, 10 batches | `frontier_dominated`, planner 1 proposal/2 players, witness 2 proposals/4 players |

V1 값은 production SLO가 아니다. 현재 deterministic algorithm의 구조적 regression budget이며 policy, generator 또는 objective를 의도적으로 바꾸면 diff에서 metric tradeoff를 설명하고 이 문서와 test를 같은 change에서 갱신한다.

## Timing Evidence

`scripts/check.sh`는 planner/engine benchmark를 `-benchtime=1x`로 실행해 경로가 계속 측정 가능한지 확인한다. elapsed time과 allocation은 machine, toolchain과 background load에 따라 달라지므로 repository gate의 고정 threshold로 사용하지 않는다.

production cycle p95, allocation ceiling과 capacity target은 실제 consumer workload, target hardware와 반복 benchmark history가 생기는 P10에서 결정한다.

## Verification

- deterministic gate: `go test ./internal/lab -run 'TestRunFullCorpusPreservesCoverageAndOrdering|TestRunReportsSyntheticMetricsAndOracleGap|TestRunReportsBatchQualityFrontier'`.
- planner benchmark: `go test ./internal/planner -run '^$' -bench '^BenchmarkPlan' -benchtime=1x`.
- engine benchmark: `go test ./internal/engine -run '^$' -bench '^BenchmarkEngine' -benchtime=1x`.
