# Project Status

## Current Milestone

P0부터 P11 interactive flow visualization까지 완료되었다. source/service는 계속 experimental alpha이며 stable v1 release는 명시적인 blocker가 해결될 때까지 gate가 차단한다.

## Established

- 프로젝트 이름과 repository identity: Sema.
- domain vocabulary: `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`.
- output contract: 한 cycle에서 ticket이 겹치지 않는 여러 match proposal.
- reference workloads: 2:2부터 50:50 team match, 총원 100명의 duo/squad battle royale.
- objective schedule: skill balance와 role composition에서 wait time과 latency 쪽으로 시간 기반 완화.
- consistency baseline: per-aggregate revision, reserve/commit CAS, in-process coordinator authority.
- implementation baseline: Go, 하나의 deployable process, in-memory core와 durable service journal.
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
- checksummed `sema-journal-v1`과 sync-before-success durable runtime.
- active reservation, assignment, acknowledgment와 policy/ticket restart replay.
- complete plan decision audit와 snapshot ID idempotency, torn-tail recovery, corruption refusal와 single-writer lock.
- explicit `v0alpha1` HTTP DTO와 policy/ticket/backfill/plan/reservation/assignment endpoint.
- server-owned clock, durable proposal ID authority와 restart-safe synchronous/polling delivery.
- loopback-default `sema-server`, bounded strict JSON, typed failure mapping과 graceful shutdown.
- low-cardinality Prometheus metrics, W3C request trace와 liveness/readiness endpoint.
- raw payload를 제외한 paged durable decision audit summary.
- 실제 HTTP lifecycle 부하, process restart와 incomplete journal tail 복구를 묶은 격리형 operational validator.
- pinned non-root `scratch` image, host-loopback Compose example과 single-writer operations runbook.
- 2 CPU/2 GiB reference container의 repeated service SLO와 Go latency/allocation budget, sanitized CI history artifact.
- full/container/performance/recovery/publication 검증을 묶고 현재 v1 stable tag를 차단하는 release admission gate.
- 실제 loopback HTTP lifecycle을 단계별로 실행하는 deterministic mixed-party Flow simulator.
- Unicode party movement, proposal/reservation/assignment/departure와 ASCII/reduced-motion fallback을 제공하는 `cmd/sema-tui`.
- waiting, active와 departed surface를 함께 검증하는 terminal-independent Flow snapshot.
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
- external database, journal compaction와 multi-replica coordination.
- authentication/TLS/rate limit, telemetry backend/alerts와 authenticated remote deployment.
- stable/v1 Go API, stable production wire protocol과 실제 external consumer evidence.
- stable release 자체; 현재 `stable_admitted: false`다.
- production cycle scheduler, external producer를 포함한 shared queue observer와 authenticated event stream.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 현실적인 existing-roster backfill 분포.
- append-only journal에는 아직 compaction, online backup와 numeric recovery SLO가 없다.

## Next Slice

P11 repository-owned 목표는 완료되었다. 다음 장기 slice는 실제 consumer와 production target이 생겼을 때 authentication/TLS gateway, stable API, production traffic calibration과 external transactional authority 중 필요한 항목을 evidence에 따라 연다. 그 전에는 Flow demo를 production scheduler로, reference SLO를 제품 SLA로 승격하지 않는다.
