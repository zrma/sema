# Runtime Validation

## Scope

현재 runtime evidence는 same-process `internal/engine` boundary를 대상으로 한다. 각 benchmark iteration은 새 engine 생성, policy registration, ticket ingestion, plan, 모든 proposal의 reserve와 confirm을 포함한다. fixture 생성과 외부 network/storage는 측정 경로에 포함하지 않는다.

이 evidence는 구현 간 비교와 구조적 병목 탐색에 사용한다. machine-specific elapsed time이나 allocation 수치를 제품 SLO로 기록하지 않는다.

## Workload Matrix

- reference: 2:2 solo, 50:50 solo, 100-player battle royale duo.
- queue: 5:5 solo policy에서 100, 500, 1000 active ticket과 proposal limit 1.
- scale probe: 5:5 solo policy에서 10K/100K queue의 unbounded와 256-ticket discovery window.
- failure: reservation expiry, concurrent terminal acknowledgment, process restart와 producer replay.

## Decision Audit Vocabulary

| Metric | Meaning |
|---|---|
| `proposals/op` | 한 cycle에서 생성된 mutually disjoint proposal 수 |
| `matched_tickets/op` | proposal에 포함된 match ticket 수; player 수와 구분 |
| `unmatched_tickets/op` | cycle 뒤 남은 match ticket 수 |
| `unmatched_<reason>/op` | stable `UnmatchedReason`별 남은 ticket 수 |
| `search_nodes/op` | proposal evidence에 기록된 탐색 node 합계 |
| `candidate_tickets/op` | proposal search window에 전달된 match ticket 합계 |
| `truncated_candidate_windows/op` | fitting supply가 policy window 뒤에 남은 proposal 수 |
| `budget_exhausted/op` | candidate/node budget truncation을 기록한 cycle 비율 |
| `pending_assignments/op` | reserve와 confirm을 통과해 pending이 된 assignment 수 |

benchmark metric은 aggregate evidence이며 proposal의 `ScoreEvidence`, batch의 unmatched records와 assignment read model이 상세 source of truth다. lifecycle outcome은 `pending`, `completed`, `cancelled`, `failed` 상태와 typed `FailureCode`를 사용한다.

## Failure Invariants

- reservation이 만료되면 proposal의 모든 ticket이 함께 release되고 다음 cycle에 다시 보인다.
- concurrent terminal acknowledgment는 정확히 하나만 성공하고 assignment read model에는 하나의 terminal outcome만 남는다.
- 새 process는 active demand, reservation, assignment를 복구하지 않는다. producer replay 뒤 fixed snapshot은 같은 proposal을 만든다.

## Commands

- focused test와 race: `go test ./internal/engine`, `go test -race ./internal/engine`.
- runtime benchmark: `go test ./internal/engine -run '^$' -bench '^BenchmarkEngine' -benchtime=1x`.
- planner scale: `go test ./internal/planner -run '^$' -bench '^BenchmarkPlanLargeQueues$' -benchtime=1x -count=1`.
- full repository gate: `scripts/check.sh`.
