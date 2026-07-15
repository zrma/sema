# Sema

Sema는 플레이어와 파티를 제약 조건에 맞는 게임 세션으로 구성하는 multiplayer match composition engine이다.

이름은 서로 다른 두 영역을 하나로 묶는 고대 이집트의 [Sema-Tawy](https://www.metmuseum.org/art/collection/search/570445) 상징에서 가져왔다. 저장소 이름은 은유를 제공하지만 코드와 API는 `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`처럼 업계에서 통용되는 명시적 용어를 사용한다.

## Status

P0 architecture foundation, P1 objective policy, P2 assignment lifecycle, transport-neutral application engine, same-process adapter, runtime validation과 demand index가 완료되었다. 현재 구현은 deterministic bounded candidate ranking부터 terminal assignment acknowledgment까지 하나의 in-process facade로 실행 가능하며, 다음 milestone은 policy content identity다.

## Design Direction

- 새 매치 요청과 기존 세션의 backfill 수요를 하나의 탐색 모델에서 다룬다.
- 한 matchmaking cycle에서 ticket이 겹치지 않는 여러 `MatchProposal`을 `ProposalBatch`로 반환한다.
- candidate discovery, hard constraints, scoring, proposal, reservation, commit을 분리한다.
- 탐색기는 side effect 없는 deterministic core로 유지한다.
- 결과뿐 아니라 선택 이유와 탈락 이유를 설명 가능한 evidence로 남긴다.
- 정책은 교체 가능하게 만들고 orchestration과 저장소 구현에 결합하지 않는다.

## Repository Map

- `AGENTS.md`: 짧은 AI-first bootstrap map과 GPT-5.6 baseline.
- `docs/agent-harness.md`: 자율 실행, 검증, 권한, 에스컬레이션 계약.
- `docs/HANDOFF.md`: 무컨텍스트 작업 시작점.
- `docs/status.md`: 현재 구현 상태와 리스크.
- `docs/roadmap.md`: milestone 순서와 완료 기준.
- `docs/architecture.md`: 초기 시스템 경계와 핵심 invariant.
- `docs/domain-model.md`: entity identity, freshness, ownership 계약.
- `docs/lifecycle.md`: ticket, reservation, assignment 전이와 typed failure.
- `docs/reference-workloads.md`: 대표 match size와 party distribution.
- `docs/reference-scenarios.md`: 실행 가능한 correctness와 benchmark fixture.
- `docs/decisions/`: 확정된 architecture decision.
- `docs/todo-*/`: 완료 evidence와 현재 milestone을 담는 작업 계약.
- `docs/REPO_MANIFEST.yaml`: repository entrypoint와 검증 명령.

## Local Verification

```sh
scripts/check.sh
```

로컬 change 관리는 `jj`를 사용한다. push, tag, release와 visibility 변경은 별도 외부-write 권한 경계다.
