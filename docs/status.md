# Project Status

## Current Milestone

`P0: Architecture Foundation`을 시작할 수 있는 repository bootstrap이 완료되었다.

## Established

- 프로젝트 이름과 repository identity: Sema.
- domain vocabulary: `MatchTicket`, `BackfillTicket`, `MatchProposal`, `Reservation`, `Assignment`.
- planner와 coordinator의 side-effect boundary.
- GPT-5.6 `agent-harness-v1`, local validation, publication boundary contract.
- gitignore.io 기반 OS/editor/VCS baseline과 local secret/artifact overlay.
- 로컬 change management는 `jj`; push는 명시적 권한 경계.

## Not Implemented

- executable package와 implementation language.
- canonical domain schema와 lifecycle tests.
- candidate index, constraint engine, scorer, optimizer.
- reservation/assignment persistence와 distributed coordination.
- API, observability, benchmark, deployment.

## Risks And Decisions Pending

- match quality, queue latency, compute budget의 objective 우선순위.
- consistency와 failure recovery 수준.
- workload fixture 없이 구현 언어와 topology를 먼저 선택할 위험.
- public repository 전환 여부와 remote identity.

## Next Slice

`docs/todo-0001-foundation/spec.md`에 따라 domain contract와 deterministic reference scenarios를 고정한다.
