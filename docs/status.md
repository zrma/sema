# Project Status

## Current Milestone

P0부터 P7 deterministic evaluation과 scalable candidate discovery baseline이 완료되었다. 현재는 P8 public integration contract를 준비하며 실제 consumer가 생길 때 production-facing boundary와 compatibility를 재평가한다.

## Established

- 프로젝트 이름과 repository identity: Sema.
- domain vocabulary: `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`.
- output contract: 한 cycle에서 ticket이 겹치지 않는 여러 match proposal.
- reference workloads: 2:2부터 50:50 team match, 총원 100명의 duo/squad battle royale.
- objective schedule: skill balance와 role composition에서 wait time과 latency 쪽으로 시간 기반 완화.
- consistency baseline: per-aggregate revision, reserve/commit CAS, in-process coordinator authority.
- implementation baseline: Go, 하나의 deployable process, 인메모리 상태.
- canonical entity schema, lifecycle, typed failure contract.
- immutable snapshot과 party-preserving bounded enumeration.
- deterministic multi-match와 backfill-first `ProposalBatch`.
- in-memory coordinator의 revision/roster CAS, atomic fixed-TTL reservation, idempotent assignment.
- 2:2부터 50:50, 100인 duo/squad, stale/conflict/expiry/concurrency reference test.
- Go format, module hygiene, vet, test, race detector, planner benchmark가 포함된 local/CI gate.
- hard constraint와 soft objective package boundary.
- versioned role requirement와 wait-based skill/role relaxation.
- best-known bounded candidate ranking과 replayable objective evidence.
- stable unmatched reason과 100/500/1000 ticket queue benchmark.
- pending/complete/cancel/fail assignment state와 idempotent acknowledgment.
- backfill expected/resulting roster version CAS handoff와 stale failure outcome.
- ingestion부터 terminal assignment까지 조합하는 `internal/engine` facade와 end-to-end fixture.
- direct engine call, producer replay, synchronous acknowledgment, single-replica runtime adapter baseline.
- process restart 뒤 empty state와 active-demand replay를 실행하는 deterministic engine fixture.
- ingestion부터 pending assignment까지 실행하는 reference/queue engine benchmark와 decision-audit metric vocabulary.
- reservation expiry whole-proposal release와 concurrent terminal acknowledgment single-winner fixture.
- active ticket player ownership index와 atomic higher-revision replacement/cleanup fixture.
- canonical policy fingerprint와 snapshot/policy/placement-aware proposal identity.
- explicit registration, defensive read와 version conflict를 제공하는 process-local policy catalog.
- side-effect-free multi-policy simulation과 canonical coverage/quality summary.
- public repository identity `github.com/zrma/sema`와 Apache-2.0 source license.
- public source이지만 Go package는 `internal/`에 유지하고 public API compatibility는 아직 약속하지 않는 경계.
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- built-in team/battle-royale/backfill/no-match/objective corpus를 실행하는 `cmd/sema-lab`.
- ticket/player coverage, unmatched reason, search evidence와 proposal placement를 제공하는 deterministic text report.
- seeded party/skill/role/latency/wait snapshot generator와 player coverage/oldest-unmatched-wait metric.
- 12 ticket 이하 new-match의 exhaustive single-proposal oracle와 bounded quality-gap evidence를 제공하는 experimental `v0alpha2` JSON report.
- point-estimate rating boundary와 deterministic coverage/search/oracle regression budget.
- versioned candidate ticket window, discovery truncation evidence와 oldest-prefix quality tradeoff.
- 10K correctness, 10K/100K benchmark gate와 planner invariant fuzz target.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- production-calibrated arrival sequence와 rating uncertainty/confidence model.
- region/skill/role-specific candidate index와 full unmatched output pagination.
- reservation/assignment persistence와 distributed coordination.
- API/server entrypoint, observability, deployment.
- import 가능한 public Go SDK와 compatibility/migration policy.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 현실적인 existing-roster backfill 분포.
- 인메모리 baseline은 process restart recovery를 제공하지 않는다.

## Next Slice

same-process, producer replay, synchronous acknowledgment와 single-replica baseline을 유지한다. 다음 slice는 P8 실제 reference consumer와 최소 `v0alpha` integration contract다. 외부 consumer가 아직 없으면 먼저 repository-owned example consumer로 package/CLI boundary를 검증하되 stable compatibility나 service transport는 약속하지 않는다.
