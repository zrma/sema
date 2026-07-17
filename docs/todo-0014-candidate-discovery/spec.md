# P7 Candidate Discovery And Scale Spec

- Status: Complete

## Objective

large queue에서 exact placement search의 입력을 deterministic하게 제한하고, approximation의 coverage/fairness/quality 영향과 10K/100K capacity evidence를 숨김없이 노출한다.

## Deliverables

- `internal/discovery` queue-prefix candidate window boundary.
- versioned `MaxCandidateTickets` policy limit과 fingerprint identity.
- candidate ticket count/window truncation proposal evidence와 batch budget outcome.
- 10K correctness fixture와 10K/100K benchmark matrix.
- bounded-window quality-gap oracle diagnostic과 planner invariant fuzz target.

## Acceptance

- zero window limit은 기존 unbounded result를 유지한다.
- positive limit은 oldest fitting tickets만 선택하고 oversized backfill party가 quota를 막지 않는다.
- window truncation은 proposal evidence, `SearchTruncated`와 batch `BudgetExhausted`에 나타난다.
- policy window 변경은 fingerprint와 proposal identity를 바꾼다.
- 10K queue에서 exact team capacity, disjoint placement, full ticket coverage와 256-ticket evidence가 유지된다.
- diagnostic은 oldest-window planner quality와 더 좋은 oracle vector를 함께 기록한다.
- input-order determinism, input immutability, capacity, disjoint와 full ticket accounting fuzz invariant가 통과한다.
- focused/race/full repository gate가 100K benchmark path까지 실행한다.

## Out Of Scope

- game/region-specific skill, role와 network bucket index.
- `ProposalBatch.Unmatched`를 summary/cursor API로 변경.
- distributed queue partition과 horizontal worker ownership.
- machine-specific latency/allocation SLO.

## Completion Evidence

- `internal/discovery.SelectWindow`가 canonical fitting prefix와 truncation을 반환한다.
- planner가 candidate-window/exact-candidate/node budget을 구분해 evidence와 failure reason에 반영한다.
- 10K normal test와 10K/100K unbounded/window benchmark가 large-queue path를 실행한다.
- candidate-window oracle fixture와 seeded fuzz corpus가 approximation/invariant를 고정한다.

상세 contract와 tradeoff는 `docs/candidate-discovery.md`, architecture decision은 ADR 0008이 소유한다.
