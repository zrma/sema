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
- [x] Unicode/color, ASCII, medium/tall/compact terminal regression gate
