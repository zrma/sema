# Project Status

## Current Milestone

`P0: Architecture Foundation`, `P1: Objective Policy`, `P2: Assignment Lifecycle`이 완료되었다. 현재 milestone은 `P3: Application Runtime`이며, 검증된 package를 transport-neutral application service로 조합한다.

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
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- 대규모 queue를 위한 candidate index와 partition.
- global optimum 또는 품질 근사 보장.
- reservation/assignment persistence와 distributed coordination.
- API/server entrypoint, observability, deployment.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 backfill fixture의 P0 범위.
- 인메모리 baseline은 process restart recovery를 제공하지 않는다.
- public repository 전환 여부와 remote identity.

## Next Slice

`docs/todo-0004-application-runtime/spec.md`에 따라 package orchestration facade와 end-to-end application fixture를 구현한다.
