# Matcher V0 Conformance Matrix

## Scope

이 matrix는 matcher core가 service productization 중에도 보존해야 할 executable contract를 모은다. `scripts/check.sh`가 deterministic corpus, persisted fuzz corpus, race와 benchmark smoke를 실행하며, 장시간 random fuzz는 algorithm change의 focused validation으로 추가 실행한다.

## Matrix

| Contract | Executable evidence | Exit condition |
|---|---|---|
| deterministic replay와 input immutability | `FuzzPlanInvariants`, `TestPlanReturnsDeterministicDisjointMatches`, `TestComposeProvidesDeterministicExternalSurface` | 같은 snapshot/policy와 input permutation이 같은 batch를 반환하고 caller input을 변경하지 않는다. |
| party integrity, capacity와 disjoint batch | `FuzzPlanInvariants`, `TestPlanPreservesParties`, `TestBatchFrontierRejectsConflictingPlannerBatch` | party를 쪼개지 않고 team capacity를 채우며 ticket/backfill target을 batch 안에서 재사용하지 않는다. |
| hard admissibility와 stable unmatched evidence | `TestPlanKeepsHardConstraintFailuresUnmatched`, `TestPlanReportsHardRoleReason`, `TestPlanReportsStableUnmatchedReasons` | hard latency/role/capacity failure는 candidate 밖에 남고 stable reason으로 설명된다. |
| global batch quality와 small-queue frontier | `TestSelectProposalBatchMatchesExhaustiveOracle`, `TestParetoSelectionRepairsDominatedRankSum`, `TestBatchFrontierDefaultBudgetDifferentialCorpus` | default small boundary에서 planner batch가 exhaustive Pareto frontier와 equivalent다. |
| explicit approximation과 truncation | `FuzzPlanInvariants`, `TestPlanReportsSearchBudgetExhaustion`, `TestPlanExposesCandidateTruncation`, `TestPlanReturnsFeasibleBatchWhenGlobalSelectionBudgetEnds` | generation/window/selection truncation을 숨기지 않고 `BudgetExhausted`와 best-feasible batch를 함께 반환한다. |
| wait-priority fairness | `TestPlanBoundsWaitPriorityUnderSustainedArrivals`, `TestSelectionServesOldestPriorityDemandBeforeRankSum`, `TestPlanUsesBackfillDemandAgeForPriorityEvidence` | non-truncated graph의 같은 backfill tier에서 configured priority age에 도달한 oldest feasible demand가 fresh rank utility에 영구히 밀리지 않는다. |
| roster-aware backfill quality와 freshness | `TestPlanBalancesResultingBackfillRoster`, `TestBatchFrontierUsesRosterAwareBackfillQuality`, `TestReserveRejectsStaleBackfillRoster` | incoming placement는 versioned existing roster aggregate와 함께 평가되고 stale revision/roster proposal은 atomic하게 거부된다. |
| indexed discovery equivalence | `FuzzIndexedWindowEquivalent`, `TestIndexedWindowMatchesLinearSmallShapes`, `TestIndexedWindowMatchesLinearTenThousandTickets` | indexed lookup의 ticket order, empty shape와 truncation이 linear oldest-fitting prefix와 `DeepEqual`이다. |
| reservation/assignment CAS | `TestReserveRejectsStaleTicketRevisionAtomically`, `TestConcurrentReserveHasOneWinner`, `TestBackfillAcknowledgmentValidatesRosterCAS` | proposal은 freshness를 다시 확인하고 concurrent ownership과 terminal transition은 single-winner다. |
| public alpha conversion과 migration | `TestAPIVersionMarksRosterAwareBackfillMigration`, `TestComposeUsesRosterVersionedBackfillQuality`, `TestComposeCopiesInputAndExposesCandidateWindow` | public `alpha`가 internal type을 누출하지 않고 current marker와 migration evidence를 보존한다. |
| performance path availability | `BenchmarkPlan`, `BenchmarkEngine`, `BenchmarkBuildIndex`, `BenchmarkWindowSelectionReuse` | single-select, large queue, engine과 reusable index 경로가 측정 가능하고 stateless plan이 index build cost를 숨기지 않는다. |

## Fuzz Contract

repository gate는 seed와 발견된 regression corpus를 항상 재실행한다. matcher algorithm 또는 candidate index를 바꾸는 change는 다음 focused fuzz도 실행한다.

```sh
go test ./internal/planner -run '^$' -fuzz '^FuzzPlanInvariants$' -fuzztime=3s
go test ./internal/discovery -run '^$' -fuzz '^FuzzIndexedWindowEquivalent$' -fuzztime=3s
```

planner fuzz는 multi-proposal upper bound, optional roster-aware backfill, input permutation, party/capacity/disjointness와 truncation evidence를 함께 검증한다. discovery fuzz는 party shape와 candidate limit을 바꾸면서 linear/indexed output의 exact equality를 검증한다.

## Ownership Boundary

### Matcher-Owned And Closed

- immutable deterministic planning input과 canonical output order.
- party/capacity/hard constraint admissibility.
- time relaxation과 wait-priority fairness semantics.
- diverse candidate generation, disjoint multi-proposal selection과 small-queue Pareto relation.
- roster-aware backfill quality, revision/roster freshness와 reservation CAS.
- candidate window/index의 exact semantics, explicit approximation과 replayable evidence.

### Consumer Calibration-Owned

- production MMR, uncertainty/confidence와 team balance metric.
- 실제 traffic, arrival, party-size와 existing-roster distribution.
- numeric wait, latency, planning-cycle와 quality/SLO threshold.
- game-specific role taxonomy, multi-role semantics와 hard/soft requirements.
- region/geography model과 incomparable Pareto point 사이의 product preference.

### Service Productization-Owned

- incremental candidate index lifetime과 transactional demand repository.
- queue, reservation, assignment와 audit의 durable source of truth.
- single/multi-writer 및 replica consistency.
- authenticated versioned API, pagination, delivery/streaming, quota와 tenancy.

calibration input이 생기면 policy와 fixtures는 바뀔 수 있지만 이 matrix의 구조적 invariant를 조용히 약화하지 않는다. invariant 자체를 바꾸려면 migration과 replacement evidence를 같은 change에 둔다.
