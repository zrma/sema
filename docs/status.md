# Project Status

## Current Milestone

`P0: Architecture Foundation`의 product와 implementation baseline이 확정되었고 domain schema 및 Go vertical slice를 시작할 수 있다.

## Established

- 프로젝트 이름과 repository identity: Sema.
- domain vocabulary: `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`.
- output contract: 한 cycle에서 ticket이 겹치지 않는 여러 match proposal.
- reference workloads: 2:2부터 50:50 team match, 총원 100명의 duo/squad battle royale.
- objective schedule: skill balance와 role composition에서 wait time과 latency 쪽으로 시간 기반 완화.
- consistency baseline: per-aggregate revision, reserve/commit CAS, in-process coordinator authority.
- implementation baseline: Go, 하나의 deployable process, 인메모리 상태.
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- Go module과 executable package.
- canonical domain schema와 lifecycle tests.
- candidate index, constraint engine, scorer, optimizer.
- reservation/assignment persistence와 distributed coordination.
- API, observability, benchmark, deployment.

## Risks And Decisions Pending

- matchmaking cycle p95, maximum queue wait, absolute latency cap의 수치.
- skill uncertainty와 team balance metric.
- role composition의 hard/soft 경계.
- mixed-party battle royale과 backfill fixture의 P0 범위.
- 인메모리 baseline은 process restart recovery를 제공하지 않는다.
- public repository 전환 여부와 remote identity.

## Next Slice

`docs/todo-0001-foundation/spec.md`에 따라 domain schema와 lifecycle을 고정하고 Go 인메모리 vertical slice를 구현한다.
