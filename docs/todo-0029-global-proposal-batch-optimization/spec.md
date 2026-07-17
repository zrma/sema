# P18 Global Proposal Batch Optimization Spec

- Status: Completed

## Objective

한 cycle에서 개별적으로 가장 좋은 proposal을 하나 선택하고 사용 ticket을 제거하는 반복을 없앤다. 동일 snapshot에서 다양한 admissible proposal 후보를 먼저 만든 뒤, 서로 겹치지 않는 후보 집합의 총 utility가 가장 높은 `ProposalBatch`를 선택한다. `MaxProposals`는 채워야 하는 개수가 아니라 반환 상한이다.

## Optimization Contract

candidate graph의 각 vertex는 개별 hard constraint와 현재 relaxation quality threshold를 통과한 하나의 `MatchProposal`이다. 두 vertex가 같은 `MatchTicket`을 사용하거나 같은 `BackfillTicket`을 대상으로 하면 동시에 선택할 수 없다.

선택기는 다음 순서의 deterministic objective를 사용한다.

1. 선택한 backfill proposal 수를 최대화한다.
2. 선택한 proposal의 rank utility 합을 최대화한다.
3. 동률이면 proposal 수와 canonical candidate key로 안정적으로 결정한다.

rank utility는 admissible match 하나를 성립시키는 admission baseline과 현재 candidate graph 안에서 기존 per-proposal objective ordering을 보존하는 quality rank의 합이다. baseline은 `MaxProposals` 범위에서 proposal 수가 하나 더 많은 feasible batch가 quality rank만으로 밀리지 않게 한다. 따라서 임계 통과 후보로 만들 수 있는 match 수를 먼저 보존하고 같은 수 안에서 total quality rank를 최적화한다. 같은 quality vector는 같은 utility를 받고, 더 좋은 candidate는 더 높은 utility를 받는다. 이 값은 snapshot/policy/candidate budget이 달라진 실행 사이에서 직접 비교하는 제품 quality score가 아니다.

## Candidate Generation

- canonical queue와 P7 candidate ticket window를 그대로 사용한다.
- side effect 없는 greedy cover를 seed로 넣어 현재 planner가 만들 수 있던 disjoint feasible batch를 candidate graph의 초기 incumbent로 보존한다.
- 전체 window의 각 ticket을 required anchor로 삼아 해당 ticket을 포함하는 best admissible placement를 추가한다.
- 같은 kind, backfill target과 ticket 집합을 쓰는 후보는 하나로 deduplicate하고 team placement 중 기존 objective가 가장 좋은 것을 보존한다.
- backfill candidate를 먼저 생성하되 new-match 후보도 같은 최종 selector에 전달한다.
- 이 단계는 diverse bounded approximation이며 snapshot 전체 feasible placement를 모두 생성한다고 주장하지 않는다.

## Budgets And Evidence

| Policy field | Bounded unit |
|---|---|
| `MaxCandidateTickets` | 한 placement search에 전달하는 oldest fitting ticket window |
| `MaxCandidatesPerProposal` | 한 unanchored/anchored search가 평가하는 exact placement |
| `MaxSearchNodes` | candidate generation 전체 recursive node |
| `MaxBatchCandidates` | deduplicate 뒤 selector에 전달하는 proposal candidate |
| `MaxBatchSearchNodes` | weighted set-packing branch-and-bound node |

0은 각 구현 기본값을 사용한다. 모든 값은 policy fingerprint에 포함된다.

proposal `ScoreEvidence.SelectionUtility`는 선택 당시의 상대 utility를 남긴다. batch evidence는 candidate/selected proposal 수, selected backfill 수, total utility, generation/selection node 수와 각 truncation flag를 남긴다. 어느 단계든 잘리면 `BudgetExhausted`가 true이며 selector는 빈 결과를 강제하지 않고 현재 best feasible incumbent를 반환한다.

`MaxProposals`에 도달한 남은 ticket은 `proposal_limit`, budget 때문에 결론이 잘린 경우는 `search_budget`, admissible 대안이 있었지만 더 높은 batch objective에 포함되지 않은 경우는 `batch_objective`를 대표 unmatched reason으로 사용한다.

## Acceptance

- 한 greedy-best 후보보다 서로 충돌하지 않는 두 후보의 utility 합이 높으면 두 후보를 선택한다.
- `MaxProposals == N`이어도 더 높은 objective가 하나의 후보만 선택하는 경우 하나만 반환할 수 있다.
- 하나의 match ticket과 backfill target은 batch 안에서 중복되지 않는다.
- candidate/selection input order가 바뀌어도 같은 canonical 결과를 만든다.
- 작은 candidate graph에서 branch-and-bound 결과가 exhaustive set oracle과 같다.
- selection budget이 끝나도 feasible incumbent와 truncation evidence를 반환한다.
- public Go alpha는 objective change를 `v0alpha2` migration으로 표시하고 HTTP DTO와 durable plan replay가 새 policy/evidence field를 보존한다.
- Flow의 frontend-owned game execution과 reservation/confirm lifecycle은 변경하지 않는다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- candidate graph 안에서는 selection budget이 충분할 때 exact weighted set-packing 결과다.
- candidate graph 자체는 greedy cover와 anchored best placement로 만든 bounded approximation이다.
- backfill-first는 기존 제품 계약을 보존하기 위한 lexicographic priority이며 rank utility로 상쇄되지 않는다.
- rank utility와 synthetic Flow 결과는 production-calibrated matchmaking quality, SLA 또는 traffic capacity 증거가 아니다.

## Completion Evidence

- selector fixture는 utility 10의 greedy 후보 하나 대신 충돌하지 않는 utility 6+6 후보를 선택하며 작은 exhaustive set oracle과 같은 결과를 낸다.
- selection node 1 fixture가 feasible incumbent와 `SelectionTruncated`를 보존한다.
- public `alpha.Compose`가 `v0alpha2` marker와 candidate/selected/utility evidence를 반환한다.
- 40-player 2분 Flow fixture는 기존 14 assignment, p50 wait 5초와 mean queue 7명을 유지한다.
- seed 42, 1,000-player, 30분 Flow run은 yield 9,041 bps, confirmed throughput 23,300 milli-match/min, wait p50/p90/p99 186/191/194초와 ingress lag/backlog 0을 기록했다.
- Go format/tidy/vet, focused/exhaustive fixture, full test, race, public alpha/lab/TUI/Flow smoke, release build와 planner/engine/durable benchmark를 포함한 `scripts/check.sh`가 통과했다.
- public repository publication boundary와 agent harness interface gate가 통과했다.
