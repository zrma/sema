# Sema Handoff

## Start Here

1. `AGENTS.md`와 `docs/agent-harness.md`를 읽는다.
2. `jj status`로 기존 변경과 현재 change를 확인한다.
3. `docs/status.md`, `docs/roadmap.md`, `docs/architecture.md`를 읽는다.
4. 활성 `docs/todo-*/spec.md`의 objective, acceptance, out-of-scope를 고정한다.
5. focused validation 뒤 `scripts/check.sh`로 닫는다.

## Current Baseline

- 저장소 이름과 제품 방향은 Sema로 확정했다.
- `agent-harness-v1`과 GPT-5.6 baseline을 적용했다.
- new match와 backfill을 함께 다루는 초기 architecture boundary를 문서화했다.
- 구현 baseline은 Go 단일 프로세스와 인메모리 상태다.
- 한 cycle은 서로 ticket이 겹치지 않는 여러 match를 `ProposalBatch`로 반환한다.
- 대표 workload는 2:2부터 50:50까지의 team match와 총원 100명의 duo/squad battle royale이다.
- numeric SLO, skill metric, role schema, production persistence는 아직 결정하지 않았다.
- 현재 publication class는 원격 visibility가 결정되기 전까지 `internal`이다.

## Current Work

`docs/todo-0001-foundation/spec.md`가 첫 active milestone이다. domain model, lifecycle, deterministic reference fixture를 고정하고 Go 최소 vertical slice로 검증한다.

## Completion Rule

분석이나 patch 적용만으로 완료하지 않는다. acceptance에 대응하는 fixture/test와 전체 local gate를 통과하고 status/roadmap을 현재 상태에 맞춘다. push, tag, release, visibility 변경은 별도 권한 경계다.
