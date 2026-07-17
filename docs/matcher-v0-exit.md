# Matcher V0 Exit Program

## Goal

Sema의 다음 장기 구간은 match composition core의 algorithmic boundary를 닫고, 기존 journal/HTTP prototype을 제품형 persistence와 API service로 재설계하는 milestone을 시작할 수 있는 상태를 만드는 것이다. 현재 durable runtime과 `v0alpha1` HTTP는 이 전환을 검증한 V0 prototype이며 최종 service architecture로 간주하지 않는다.

실제 traffic corpus, rating contract와 consumer SLO가 없는 항목은 임의의 제품 수치로 확정하지 않는다. 대신 deterministic fixture, exhaustive small-case evidence, bounded large-queue search와 failure semantics를 matcher exit evidence로 사용한다.

## Matcher Completion Sequence

### P24: Small-Queue Pareto Planning

- P23 exhaustive frontier를 default small-queue planner의 differential oracle로 사용한다.
- 개별 proposal rank 합이 global batch를 지배 가능한 상태로 남기지 않도록 candidate diversity와 Pareto repair를 적용한다.
- explicit approximation budget과 large/single-select fast path는 보존한다.

### P25: Queue Fairness And Starvation

- 지속 유입 중 오래된 feasible demand가 새 demand에 영구히 밀리지 않는 service invariant를 정의한다.
- wait relaxation, candidate window와 batch selection이 같은 age contract를 사용하게 한다.
- impossible demand, hard rejection과 feasible-but-delayed demand를 분리해 evidence를 남긴다.

완료된 P25 contract는 backfill-count tier 안에서 oldest eligible priority wait와 selected priority demand 수를 rank utility보다 먼저 비교한다. hard rejection은 candidate graph 밖에 남고, bounded graph에서 feasible한 priority demand와 실제 service 수는 batch evidence로 분리된다. explicit truncation은 invariant 밖이며 `BudgetExhausted`가 이를 표시한다.

### P26: Backfill Quality Context

- vacancy shape만 보는 현재 backfill을 existing roster의 skill/role/latency summary와 함께 평가한다.
- roster freshness와 quality evidence가 같은 `rosterVersion`에 묶이게 한다.
- new match와 backfill이 같은 admissibility vocabulary를 쓰되 backfill-first product priority는 명시적으로 유지한다.

완료된 P26 contract는 optional team aggregate를 backfill ticket revision과 `rosterVersion`에 묶고 incoming placement 뒤 resulting skill gap, role penalty와 latency evidence를 계산한다. context가 없으면 legacy vacancy-only behavior를 유지하며 stale revision/roster reserve는 전체 proposal을 거부한다.

### P27: Indexed Candidate Discovery

- oldest-prefix baseline과 동일한 hard/fairness contract를 보존하는 skill/role/latency partition을 비교한다.
- exhaustive small case와 large deterministic corpus에서 quality loss, search node와 truncation을 함께 측정한다.
- 실제 region contract가 생기기 전에는 임의의 geography schema를 public API에 추가하지 않는다.

완료된 P27 prototype은 party/skill/role/latency partition과 exact oldest-prefix merge를 제공한다. small/10K comparison은 linear window와 동일하고 100K benchmark는 reusable lookup 이득과 더 큰 one-time build cost를 분리한다. stateless plan에 rebuild하지 않으며 incremental index lifetime은 stateful demand repository가 소유하도록 productization entry에 넘긴다.

### P28: Matcher V0 Exit Gate

- policy, candidate discovery, batch optimization, backfill과 starvation evidence를 하나의 matcher conformance matrix로 묶는다.
- deterministic replay, input permutation, party/capacity, disjointness, budget truncation과 explanation invariant를 fuzz/property gate로 고정한다.
- 남은 matcher decision이 consumer calibration 항목뿐인지 확인하고 algorithm-owned TODO를 닫는다.

## Service Productization Entry

P28을 통과하면 다음 milestone은 matcher 알고리즘 확장이 아니라 service boundary 재설계로 시작한다. 시작 문서는 다음을 입력으로 가져야 한다.

- journal prototype에서 transactional durable authority로 가는 persistence model과 migration.
- queue/snapshot/assignment source of truth, idempotency scope와 multi-writer 여부.
- experimental HTTP DTO를 대체할 versioned resource, pagination, streaming/delivery와 error contract.
- authentication, tenancy, quota와 operational ownership.
- existing V0 journal/API compatibility를 유지할지 명시적으로 폐기할지에 대한 migration decision.

이 program의 완료는 production service 구현 완료가 아니라, matcher core가 service productization의 moving target이 아니며 위 항목을 독립 milestone로 시작할 수 있다는 evidence다.
