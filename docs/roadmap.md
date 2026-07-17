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

- [ ] 실제 reference consumer와 integration example
- [ ] 최소 `v0alpha` public Go API 또는 versioned schema
- [ ] compatibility and migration policy
- [ ] distribution and release workflow

## P9: Production Runtime

- [ ] ticket/session ingestion API
- [ ] durable reservation/assignment persistence와 restart recovery
- [ ] delivery/retry contract와 durable decision audit source of truth
- [ ] process 분리 여부와 horizontal worker coordination 재평가

## P10: Operational Validation

- [ ] metrics, traces와 decision audit exporter
- [ ] load, soak와 failure-injection validation
- [ ] container/deployment example과 operations runbook
- [ ] target hardware의 repeated latency/allocation benchmark history와 numeric SLO
- [ ] measured SLO, recovery와 stable release gate
