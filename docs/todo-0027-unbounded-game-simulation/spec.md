# P16 Matchmaker And Game Runtime Ownership Correction

- Status: Completed

## Objective

Sema가 소유하는 match composition/assignment 처리량과 frontend가 소유하는 game execution simulation을 분리한다. Flow의 진행 중 game 수는 player 상태와 return timing을 보여주는 read model일 뿐 새 proposal 생성을 막는 capacity authority가 아니다.

## Ownership Contract

- Sema는 ticket ingestion, plan, proposal, reservation과 assignment confirm까지 소유한다.
- confirm된 participant가 game을 실행하고 결과를 제출하는 구간은 frontend/game runtime 책임이다.
- Flow의 `game_duration`은 synthetic result와 ticket return timestamp를 계산할 때만 사용한다.
- 진행 중 game 수는 관측값이며 planning eligibility의 입력이 아니다.
- closed population이므로 concurrent game 수는 자연스럽게 `population / players_per_match` 이하로 bounded된다.

## API And Measurement Correction

- Flow `Config`, TUI와 report CLI에서 `MaxConcurrentMatches` / `-concurrent-matches`를 제거한다.
- `sema.flow.measurement.v0alpha3`는 game-capacity field 없이 Sema-owned batch/timing configuration만 기록한다.
- capacity matrix profile은 `matches_per_cycle` 하나만 바꾸며 schema를 `sema.flow.capacity-matrix.v0alpha2`로 올린다.
- 이전 concurrent/batch P15 matrix는 frontend game-capacity와 matchmaker batching을 섞었으므로 superseded evidence로 표시한다.

## Acceptance

- active game 수가 어떤 값이어도 충분한 queued player와 planning interval만 만족하면 plan한다.
- 기본 1,000-player Flow에서 active game이 과거 8-game cap을 넘고 lifecycle/result/return이 계속 진행된다.
- TUI Unicode/ASCII/reduced-motion snapshot이 확장된 lifecycle 수를 안전하게 요약한다.
- report와 matrix text/JSON에 game-capacity configuration이 남지 않는다.
- batch 2/4/8 multi-seed matrix가 동일 initial ingress, arrival lag 0과 final ingress backlog 0을 유지한다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Out Of Scope

- 실제 game-server allocator capacity, queue admission이나 allocation backpressure.
- frontend/game runtime API와 result ingestion production contract.
- product wait/quality target에 따른 automatic batch selection.

## Completion Evidence

- `TestSimulatorPlanningContinuesAboveEightActiveGames`가 active game 8개를 넘은 상태에서도 새 plan이 발생함을 검증한다.
- 1,000-player snapshot은 active game 18개와 계속 변하는 queue를 동시에 표시하며 `MATCH LIFECYCLE` 패널의 요약 렌더링을 유지한다.
- 30분 기본 `sema.flow.measurement.v0alpha3` report는 assignment yield 9,039 bps, confirmed throughput 23,300 milli-match/min과 ingress lag/backlog 0을 기록한다.
- batch 2/4/8, seed 42/73/101의 `sema.flow.capacity-matrix.v0alpha2`는 모든 run에서 initial ticket 600개, ingress lag/backlog 0과 `demand_comparable=true`를 기록한다.
- focused/race/full repository gate와 publication boundary를 통과했다.
