# ADR 0008: Candidate Window Baseline

- Status: Accepted

## Context

bounded placement enumeration은 1K queue까지 실행 가능하지만 10K/100K에서는 전체 queue를 search input으로 복사하고 suffix state를 만든다. 동시에 current batch contract가 모든 unmatched ticket을 반환하므로 discovery만 최적화해 전체 linear cost를 제거할 수는 없다.

## Decision

- candidate discovery를 `internal/discovery` package boundary로 분리한다.
- 첫 implementation은 canonical queue에서 oldest fitting ticket을 고르는 prefix window다.
- `MatchmakingPolicy.MaxCandidateTickets`가 search별 ticket 상한을 소유하고 zero는 unbounded다.
- window limit은 policy fingerprint와 proposal identity에 포함한다.
- window truncation, input ticket 수와 search truncation을 proposal/batch evidence로 노출한다.
- region/skill/role bucket과 full unmatched contract 변경은 실제 workload/API evidence 전까지 추가하지 않는다.

## Consequences

- 기존 policy는 zero default로 동일한 unbounded semantics를 유지한다.
- opt-in window는 oldest demand를 우선하지만 뒤쪽 quality candidate를 놓칠 수 있다.
- backfill window는 현재 vacancy에 들어갈 수 없는 party가 quota를 소비하지 않는다.
- 100K end-to-end 비용은 unmatched materialization 때문에 여전히 queue size에 선형으로 증가한다.

## Revisit Triggers

- measured queue distribution에서 prefix scan 또는 full unmatched materialization이 SLO를 넘는다.
- region/role/skill bucket이 candidate recall을 유지하면서 cost를 줄인다는 corpus evidence가 생긴다.
- external API가 full unmatched records 대신 summary, cursor 또는 delta를 허용한다.
- horizontal worker가 queue partition ownership을 요구한다.
