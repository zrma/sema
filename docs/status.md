# Project Status

## Current Milestone

P0부터 P27 indexed candidate discovery까지 완료되었다. source/service는 계속 experimental alpha이며 stable v1 release는 명시적인 blocker가 해결될 때까지 gate가 차단한다.

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
- greedy cover와 anchored search로 다양한 admissible proposal candidate를 만드는 deterministic multi-match planner.
- ticket/backfill conflict graph에서 backfill 수와 total rank utility를 최적화하는 bounded weighted set-packing `ProposalBatch`.
- `MaxProposals` 상한, candidate/selection 독립 budget, best-feasible truncation과 replayable batch evidence.
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
- global batch objective, small-queue Pareto, wait-priority service와 roster-aware backfill을 반영한 public Go alpha `v0alpha5`, `v0alpha1`부터 이어지는 migration contract.
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
- waiting, active와 completed surface를 함께 검증하는 terminal-independent Flow snapshot.
- stable identity를 가진 기본 1,000명 player와 600개 solo/duo/trio party의 closed population registry.
- empty queue에서 시작하는 deterministic party arrival과 lifecycle 중 계속되는 queue ingestion.
- matchmaking에 쓰는 visible rating과 seeded hidden true skill의 분리.
- fixed-duration 5v5 result, logistic outcome draw와 zero-sum team Elo update.
- completed assignment 뒤 분산 cooldown을 거쳐 revision을 올린 동일 party ticket의 실제 HTTP 복귀.
- population idle/queue/in-game/cooldown, rating range, percentile, histogram과 누적 result를 표시하는 Flow TUI.
- player-weighted queue wait, arrival-to-assignment yield, match throughput과 time-weighted queue saturation을 측정하는 `cmd/sema-flow-report`.
- versioned deterministic text/JSON aggregate, proposal skill-gap/latency distribution과 final visible-rating summary.
- due arrival을 presentation step과 분리해 예약된 server-clock 시각에 처리하는 measurement-safe Flow clock.
- arrival, reservation/confirmation stage, game completion과 planning eligibility 중 가장 이른 logical timestamp로만 전진하는 deterministic Flow scheduler.
- future cooldown과 due ingress backlog를 분리하고 arrival lag와 horizon backlog를 노출하며 game-capacity field를 제거한 `sema.flow.measurement.v0alpha3`.
- active game 수와 game duration을 planning eligibility에서 분리하고 assignment confirm 이후 game/result/return simulation을 frontend-owned read model로 둔 Flow ownership contract.
- active game이 과거 8-game cap을 넘겨도 planning이 계속되는 regression과 lifecycle/result/return 연출을 유지하는 TUI `MATCH LIFECYCLE` 패널.
- seed 42/73/101과 batch upper bound 2/8/32 profile을 독립 실행하고 min/median/max를 집계하는 `sema.flow.capacity-matrix.v0alpha2`.
- profile 순서와 wall-clock parallelism에 영향받지 않는 deterministic matrix, cross-profile demand comparability와 reduced real-Flow smoke.
- wide TUI의 `AVERAGE QUEUE WAIT | RATING DENSITY` analytics 행과 `COMPLETED MATCHES | EVENT STREAM` recent 행.
- assignment confirm 전 ticket을 player-weighted한 queue wait history와 1500 exact center symmetric rating-density history.
- 같은 logical timestamp를 합치고 512 sample로 제한하는 deterministic trend read model과 Unicode/color, ASCII, medium/tall/compact terminal gate.
- selected party를 match별 color/marker로 묶어 hold 뒤 오른쪽으로 이동시키고 남은 waiting row를 frame 단위로 접는 queue departure motion과 reduced-motion final-state fallback.
- 새 lifecycle block을 batch 순서대로 panel 상단에서 stagger해 펼치고 기존 block을 아래로 이동시키는 presentation-only entry motion.
- tall analytics panel의 전체 높이에 9개 rating bucket을 비례 확장하되 label과 histogram 의미를 유지하는 density scaling.
- 한 match 분량부터 partial batch를 계획하고 backlog에서 기본 32 proposals까지 반환하는 Flow admission contract와 256 configuration safety bound.
- 1,000-player warm-up 이후 20분간 89.9 match/min, 30분 누적 wait p50/p90/p99 5/9/18초를 기록한 synthetic stability evidence.
- proposal limit/capacity가 하나인 workload에서 redundant anchored search를 생략하고 multi-proposal candidate diversity는 보존하는 planner fast path.
- 기존 budget 안으로 복구된 planner 50v50, planner 100K/window-256와 engine 1,000-ticket reference benchmark.
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- built-in team/battle-royale/backfill/no-match/objective corpus를 실행하는 `cmd/sema-lab`.
- ticket/player coverage, unmatched reason, search evidence와 proposal placement를 제공하는 deterministic text report.
- seeded party/skill/role/latency/wait snapshot generator와 player coverage/oldest-unmatched-wait metric.
- 12 ticket 이하 new-match의 exhaustive single-proposal oracle와 12 match ticket/2 backfill/2 team 이하 global batch Pareto frontier를 제공하는 experimental `v0alpha5` JSON report.
- coverage/wait와 per-match quality를 scalar calibration 없이 비교하는 planner frontier relation, dominating witness와 deterministic exhaustive counters.
- default small queue에서 distinct ticket-set candidate를 확장하고 dominated rank-sum batch를 repair하는 Pareto subset selection.
- weighted mixed-party/skill/role/latency/wait와 optional backfill의 128-seed exhaustive differential corpus.
- sustained fresh arrival 중 configured priority boundary에서 oldest feasible pair를 service하는 deterministic starvation regression.
- candidate graph의 wait-priority eligible/selected demand 수와 oldest eligible/selected wait evidence.
- backfill ticket revision/roster version에 묶인 optional existing-team player/skill/role/latency aggregate.
- incoming placement 뒤 resulting roster quality를 planner와 exhaustive frontier가 함께 비교하는 regression.
- party/skill/role/latency reusable partition index와 linear oldest-prefix exact-equivalence matrix.
- 100K repeated lookup과 one-time build cost를 분리한 discovery benchmark 및 stateful ownership boundary.
- point-estimate rating boundary와 deterministic coverage/search/oracle regression budget.
- versioned candidate ticket window, discovery truncation evidence와 oldest-prefix quality tradeoff.
- 10K correctness, 10K/100K benchmark gate와 planner invariant fuzz target.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- production-calibrated outcome curve, 실제 접속률/영구 churn sequence와 rating uncertainty/confidence model.
- region/skill/role-specific candidate index, production-scale feasible candidate enumeration과 full unmatched output pagination.
- external database, journal compaction와 multi-replica coordination.
- authentication/TLS/rate limit, telemetry backend/alerts와 authenticated remote deployment.
- stable/v1 Go API, stable production wire protocol과 실제 external consumer evidence.
- stable release 자체; 현재 `stable_admitted: false`다.
- production cycle scheduler, external producer를 포함한 shared queue observer와 authenticated event stream.
- target wait/quality에 기반한 automatic planning-batch admission.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 현실적인 existing-roster backfill 분포.
- append-only journal에는 아직 compaction, online backup와 numeric recovery SLO가 없다.

## Next Slice

P27은 reusable partition index가 linear oldest-prefix와 정확히 같은 window를 반환하도록 고정하고 per-plan rebuild가 이득이 아님을 benchmark로 분리했다. 다음 matcher slice는 P28 conformance matrix와 V0 exit gate다. 통과 뒤 `docs/matcher-v0-exit.md`의 service productization entry에서 stateful index lifetime, journal과 experimental HTTP를 persistence/API 제품 경계로 재설계한다. 현재 aggregate와 synthetic fixture를 production MMR schema나 SLA로 승격하지 않는다.
