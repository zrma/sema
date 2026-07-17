# Project Status

## Current Milestone

P0부터 P8 public alpha integration과 distribution baseline까지 완료되었다. 현재는 P9 production runtime에서 durable state, restart recovery와 delivery contract를 설계한다.

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
- `github.com/zrma/sema/alpha`의 side-effect-free `Compose`와 explicit public/internal conversion boundary.
- `internal/`을 직접 import하지 않는 repository-owned `examples/compose` reference consumer.
- `v0alpha1` compatibility/migration policy와 stable API 진입 gate.
- versioned `sema-lab` cross-build/checksum script와 verified-tag GitHub Release workflow.
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
- ticket/session ingestion server, durable state, observability와 deployment.
- stable/v1 Go API, production wire protocol과 실제 external consumer evidence.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 현실적인 existing-roster backfill 분포.
- 인메모리 baseline은 process restart recovery를 제공하지 않는다.

## Next Slice

public alpha는 immutable composition에만 유지하고 coordinator lifecycle을 확장하지 않는다. 다음 slice는 P9 single-replica durable runtime이다. ticket/session ingestion, reservation/assignment 복구와 retry/audit source of truth를 먼저 고정한 뒤에만 process 분리와 horizontal coordination을 재평가한다.
