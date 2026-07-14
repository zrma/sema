# Project Status

## Current Milestone

`P0: Architecture Foundation`이 완료되었다. 현재 milestone은 `P1: Objective Policy`이며, deterministic vertical slice 위에 skill, role, wait time, latency의 명시적 scoring/relaxation contract를 추가한다.

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
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- time-dependent skill/role/wait/latency policy와 대안 후보 비교 scorer.
- 대규모 queue를 위한 candidate index와 partition.
- global optimum 또는 품질 근사 보장.
- reservation/assignment persistence와 distributed coordination.
- assignment cancellation과 backfill roster write-back.
- API/server entrypoint, observability, deployment.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 backfill fixture의 P0 범위.
- 인메모리 baseline은 process restart recovery를 제공하지 않는다.
- public repository 전환 여부와 remote identity.

## Next Slice

`docs/todo-0002-objective-policy/spec.md`에 따라 explicit objective vector, wait-based relaxation, role contract, unmatched reason을 fixture-first로 구현한다.
