# Product Roadmap

## P0: Architecture Foundation

- [x] Sema repository와 `jj` Git backend 초기화
- [x] GPT-5.6 AI-first agent harness와 publication boundary gate 적용
- [x] 초기 domain vocabulary와 component boundary 문서화
- [x] reference workload matrix와 multi-match output contract 확정
- [x] Go 단일 프로세스·인메모리 implementation baseline 결정
- [x] revision/CAS consistency 기본값 결정
- [x] canonical entity schema와 lifecycle 정의
- [x] executable new-match와 backfill reference fixture 정의
- [x] Go 최소 vertical slice 구현
- [x] public repository identity와 Apache-2.0 publication baseline

## P1: Deterministic Match Core

- [x] immutable matchmaking snapshot
- [x] deterministic queue ordering과 bounded enumeration
- [x] party, capacity, absolute latency hard constraint evaluation
- [x] time-dependent soft objective scoring과 explanation
- [x] deterministic multi-match `ProposalBatch`와 replay test

## P2: Reservation And Assignment

- [x] proposal reserve, confirm, cancel, expiry lifecycle
- [x] idempotent fixed-TTL reservation
- [x] revision/roster conflict detection과 atomic retry boundary
- [x] assignment commit
- [x] assignment completion과 cancellation acknowledgment
- [x] backfill roster CAS handoff

## P3: Runtime Baseline

- [x] transport-neutral application engine과 end-to-end lifecycle
- [x] same-process Go adapter와 producer replay recovery boundary
- [x] in-process full-lifecycle benchmark와 failure boundary fixture
- [x] active-demand player ownership index
- [x] same-process, producer replay, synchronous acknowledgment와 single-replica integration baseline

## P4: Policy Baseline

- [x] policy content fingerprint와 replay identity
- [x] versioned policy contract
- [x] rule simulation과 offline evaluation
- [x] remote Go module identity와 internal-only package boundary

## P5: Executable Evaluation Lab

- [x] `cmd/sema-lab`과 built-in workload discovery
- [x] team, battle royale, backfill, no-match와 objective corpus
- [x] ticket/player coverage, unmatched reason와 search/quality evidence
- [x] deterministic text/detail과 experimental `v0alpha1` JSON report
- [x] focused test와 command smoke를 repository gate에 편입

## P6: Workload And Quality Evidence

- [x] seeded snapshot-level wait, party, point-skill, role scarcity와 latency model
- [x] player coverage, oldest unmatched wait와 proposal quality metric vocabulary
- [x] small-case exhaustive new-match oracle와 bounded quality-gap 측정
- [x] point-estimate rating boundary와 uncertainty revisit trigger 결정
- [x] deterministic coverage/search/oracle regression baseline

## P7: Scalable Candidate Search

- [x] queue-prefix candidate discovery/partition boundary
- [x] 10K correctness와 10K/100K ticket queue benchmark
- [x] bounded window approximation의 quality/fairness degradation 측정
- [x] invariant property/fuzz test와 algorithmic performance evidence

## P8: Public Integration Contract

- [x] repository-owned reference consumer와 integration example
- [x] 최소 `v0alpha1` public Go composition API
- [x] compatibility and migration policy
- [x] distribution build와 release workflow baseline

## P9: Production Runtime

- [x] versioned ticket/session ingestion API
- [x] durable reservation/assignment persistence와 restart recovery
- [x] restart-safe retry contract와 durable decision audit source of truth
- [x] process 분리 여부와 horizontal worker coordination 재평가

## P10: Operational Validation

- [x] metrics, traces와 redacted decision audit exporter
- [x] load, soak와 failure-injection validation
- [x] container/deployment example과 operations runbook
- [x] reference container target profile의 repeated latency/allocation history와 numeric SLO
- [x] measured SLO, recovery와 stable release admission gate

## P11: Interactive Flow Visualization

- [x] deterministic mixed-party HTTP lifecycle simulator
- [x] Unicode party movement와 proposal/reservation/assignment animation
- [x] pause, single-step, speed와 reduced-motion control
- [x] ASCII compatibility glyph와 deterministic snapshot mode
- [x] renderer width, lifecycle ordering과 command smoke gate

## P12: Closed-Loop Population Simulation

- [x] stable identity를 가진 기본 1,000명 mixed-party population
- [x] empty queue에서 시작하는 sequential arrival과 concurrent match flow
- [x] visible rating과 seeded hidden true skill 분리
- [x] fixed-duration 5v5 game과 probabilistic result
- [x] zero-sum Elo update와 분산 cooldown 뒤 revised party ticket return
- [x] rating range/histogram, idle/queue/in-game/cooldown과 completed result TUI
- [x] deterministic population, HTTP lifecycle와 terminal snapshot gate

## P13: Flow Measurement Baseline

- [x] player-weighted queue wait와 arrival-to-assignment yield
- [x] simulated-minute match throughput과 time-weighted queue saturation
- [x] proposal skill-gap/latency distribution과 final rating summary
- [x] versioned deterministic text/JSON report command
- [x] due arrival server-clock semantics와 closed-loop measurement regression gate

## P14: Discrete-Event Flow Scheduler

- [x] presentation frame과 simulated logical timestamp 분리
- [x] arrival, lifecycle operation, game completion과 planning eligibility의 next-event scheduling
- [x] due ingress 우선순위와 stable batch-stage ordering
- [x] ingress backlog, arrival lag와 horizon drain measurement contract
- [x] 동일 10분 demand horizon의 8/16/32 concurrent comparison gate

## P15: Multi-Seed Flow Capacity Matrix (Superseded)

- [x] canonical seed와 historical concurrent/batch profile matrix
- [x] 독립 run의 bounded wall-clock parallel execution
- [x] min/median/max throughput, wait, queue와 quality aggregate
- [x] cross-profile demand comparability gate
- [x] versioned deterministic text/JSON matrix command와 reference result

P15는 game-runtime capacity와 Sema planning batch를 한 profile에 섞었다. 실행 framework와 comparability contract는 유지하고 profile/result 해석은 P16이 대체한다.

## P16: Matchmaker And Game Runtime Ownership Correction

- [x] assignment confirm 이후 game 실행을 frontend/game-runtime 책임으로 명시
- [x] active game 수를 planning eligibility와 Flow configuration에서 제거
- [x] `MATCH LIFECYCLE` 관찰 패널과 synthetic result/return 연출 유지
- [x] measurement `v0alpha3`와 batch-only capacity matrix `v0alpha2`
- [x] active game 8개 초과 planning regression과 1,000-player TUI smoke

## P17: Flow Trend Panels

- [x] `COMPLETED MATCHES | EVENT STREAM` 하단 split layout
- [x] player-weighted average queue wait time-series panel
- [x] 1500 중심 symmetric rating-density time-series panel
- [x] density glyph/color intensity와 bounded logical-time sampling
- [x] match별 selection marker, queue departure와 incremental row compaction motion
- [x] Unicode/color, ASCII, medium/tall/compact terminal regression gate

## P18: Global Proposal Batch Optimization

- [x] greedy cover와 anchored search를 결합한 diverse candidate graph
- [x] 개별 admissibility를 통과한 후보만 batch selection에 전달
- [x] ticket/backfill conflict를 제한하는 weighted set-packing selector
- [x] backfill-first, total rank utility와 `MaxProposals` 상한 계약
- [x] generation/selection budget과 replayable batch evidence
- [x] greedy-failure fixture, exhaustive small oracle와 Flow regression gate

## P19: Flow Batch Admission

- [x] `matches_per_cycle`을 fixed fill target이 아닌 proposal upper bound로 적용
- [x] 한 match 분량부터 partial-batch planning 허용
- [x] 기본 32-match burst와 256-match configuration safety bound
- [x] backlog에서 한 cycle 32-match 반환 regression
- [x] 1,000-player 30분 wait/throughput 안정성 검증
- [x] TUI batch/limit/cadence 관찰값과 report/matrix 기본값 정렬

## P20: Single-Select Performance

- [x] single-proposal limit의 redundant anchored search 제거
- [x] exact one-match capacity의 redundant candidate graph 제거
- [x] multi-proposal/backfill conflict candidate graph 보존
- [x] 50v50, 100K queue와 engine 1,000-ticket 기존 budget 복구
- [x] focused/full/race/container performance와 publication gate

## P21: Flow Lifecycle Entry Motion

- [x] lifecycle block의 initial-frame pop 제거
- [x] batch proposal 순서의 top-down staggered entry
- [x] 새 row 진입에 따른 기존 lifecycle block 하향 이동
- [x] waiting/lifecycle marker와 color mapping 유지
- [x] reduced-motion final layout과 frame regression gate

## P22: Flow Rating Density Scaling

- [x] tall analytics panel의 전체 density chart 높이 사용
- [x] dynamic rating band의 비례 vertical cell 확장
- [x] bucket당 단일 Y-axis label 유지
- [x] 기존 histogram/measurement 의미 보존
- [x] 기본/tall/medium/compact와 ASCII regression gate

## P23: Batch Quality Frontier

- [x] 12 match ticket, 2 backfill ticket, 2 team exhaustive safety bound
- [x] exact-capacity new-match/backfill candidate와 disjoint batch enumeration
- [x] coverage, wait와 per-match quality의 deterministic Pareto frontier
- [x] planner equivalent/dominated/incomparable relation과 dominating witness
- [x] solo/duo/trio + backfill fixture와 bounded candidate gap diagnostic
- [x] `sema-lab` experimental `v0alpha5` text/JSON evidence와 regression gate

## P24: Small-Queue Pareto Planning

- [x] default 12-ticket/2-backfill/2-team expanded candidate path
- [x] distinct ticket-set alternative preservation과 4096 candidate safety bound
- [x] coverage tier 안의 batch Pareto dominance repair
- [x] 128-seed mixed-party/backfill exhaustive differential corpus
- [x] explicit approximation budget와 large/single-select fast path 보존

## P25: Queue Fairness And Starvation

- [x] feasible demand age/service invariant
- [x] sustained-arrival starvation diagnostic와 bounded wait evidence
- [x] relaxation, candidate window와 batch ordering의 age contract 정렬

## P26: Backfill Quality Context

- [x] roster-versioned skill/role/latency summary
- [x] new-match와 backfill의 공통 admissibility evidence
- [x] stale roster와 quality comparison fixture

## P27: Indexed Candidate Discovery

- [x] skill/role/latency partition prototype
- [x] oldest-prefix fairness와 approximation evidence 보존
- [x] small exhaustive와 large deterministic comparison

## P28: Matcher V0 Exit Gate

- [x] matcher conformance matrix와 property/fuzz gate
- [x] algorithm-owned TODO와 calibration-owned decision 분리
- [x] persistence/API service productization entry spec

## P29: Service Productization Entry

- [x] tenant-scoped adapter-neutral repository CAS와 operation receipt contract
- [x] in-memory adapter와 reusable repository conformance suite
- [x] immutable planning snapshot과 repository-versioned candidate index seam
- [x] authority/retention/failure matrix와 V0 import-only migration mapping
- [x] persistent adapter prototype와 real crash/reopen conformance
- [x] 공통 contention workload와 storage decision evidence
- [x] PostgreSQL schema, adapter와 separate-pool conformance
- [x] authenticated target API schema, pagination와 polling contract fixture
- [x] PostgreSQL primary authority와 stateless replica topology 결정

## Completed Program: Service Productization

matcher V0의 algorithm-owned contract는 완료되었다. P29가 transactional repository/resource contract, PostgreSQL adapter와 provider-neutral authenticated `v0alpha2` match-ticket vertical slice를 닫았다. PostgreSQL primary가 durable authority이고 service는 stateless replica이며 Redis는 baseline에 없다.

## P30: Authenticated Service Runtime

- [x] BackfillTicket authenticated command/read service와 atomic demand/session claim
- [x] tenant-scoped immutable Policy catalog와 authenticated command/read service
- [x] repository-versioned planning run과 immutable proposal/unmatched result page
- [x] proposal-derived reservation create/cancel/get/list와 demand claim/expiry replay
- [x] reservation confirm/assignment/acknowledgment command service
- [x] provider-neutral OIDC/JWT authenticator와 tenant/scope claim contract
- [x] provider token 기반 remote health/auth/tenant/lifecycle smoke runner
- [x] provider-neutral credential contract와 reference deployment acceptance
- [x] authenticated PostgreSQL runtime executable과 remote-listener security gate
- [x] V0 read-only import와 discard-and-retry completion marker
- [x] rollback과 backup/restore rehearsal
- [x] 실제 game traffic과 무관한 standalone product scope 명시

provider-neutral lifecycle service, optional import fixture, local backup/restore recovery rehearsal, OIDC/JWT authenticator, PostgreSQL remote executable과 redacted reference deployment acceptance가 완료되었다. 이 milestone은 실제 game traffic 전환을 의미하지 않는다.

## P31: Service Product Readiness

- [x] standalone product scope와 runtime lineage decision
- [x] PostgreSQL/OIDC 표준 command, container entrypoint와 deployment example
- [x] V0 development/reference 및 optional import compatibility surface 분리
- [x] repository-owned reference client와 wire conformance
- [x] multi-replica contention, dependency failure와 recovery matrix
- [x] workload 기반 admission, pool/timeout과 numeric service SLO
- [x] observability, backup/PITR acceptance와 stable compatibility admission decision

P31은 실제 consumer나 production traffic을 기다리는 deployment program이 아니다. 저장소가 스스로 재현할 수 있는 compatibility, availability, load/failure, observability와 recovery evidence로 PoC 형태의 V0에서 제품형 service surface로 전진했다. `sema-standard-postgres-v1`은 32 concurrent request에서 100-ticket/10-match cycle을 반복하며 explicit 64-request admission, 16/2 connection pool, 5초 operation deadline과 numeric regression budget을 고정한다. P30 wire가 첫 baseline이므로 stable admission은 완료로 가장하지 않고 별도 contract milestone로 defer했다.

## Active Program: Framework Contract Closure

`docs/development-stages.md`는 P0–P28 matcher framework core, P29–P31 reference service runtime과 P32 framework contract closure의 세 단계 lifecycle을 소유한다. P32는 실제 game integration이나 기존 service migration이 아니라 stable contract와 repository-owned cross-version evidence를 닫고 maintenance mode로 전환하는 마지막 계획 개발 단계다.

## P32: Framework Contract Closure And Maintenance Handoff

- [x] 세 단계 lifecycle과 terminal maintenance-mode 목표 문서화
- [x] stable HTTP `/v1` surface 승인과 public Go `alpha` 제외
- [x] compatibility, security exception, 180일/2개 minor deprecation window와 repository maintainer ownership 결정
- [x] `v0.3.0`/`v0.4.0` immutable tagged release 기반 previous/current client-service matrix
- [ ] migration, rollback와 end-of-support executable release gate
- [ ] stable admission과 release evidence
- [ ] maintenance-only handoff와 active development milestone 종료
