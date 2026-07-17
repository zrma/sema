# P12 Closed-Loop Population Simulation Spec

- Status: Complete

## Objective

고정된 player registry가 순차적으로 대기열에 유입되고, 5v5 matchmaking, 일정 시간의 경기, rating 갱신, cooldown과 복귀를 반복하는 deterministic closed loop를 Flow TUI에서 실행한다.

## Model

- 기본 population은 1,000명이며 모든 player의 초기 visible rating은 1500이다.
- hidden true skill은 seed로 생성한 bounded distribution이며 matchmaking 입력에는 노출하지 않는다.
- player는 반복 가능한 solo/duo/trio party로 묶이고 party는 match 사이에도 유지된다.
- 시작 시 queue는 비어 있고 stable party가 deterministic interval로 하나씩 유입된다.
- 경기 승자는 양 팀 평균 true skill 차이를 반영한 logistic probability와 seeded draw로 결정한다.
- rating은 양 팀 평균 visible rating으로 계산한 Elo expectation과 실제 승패로 갱신한다.
- 경기는 동일한 simulated duration 뒤 완료되며, 완료된 party는 deterministic cooldown 뒤 revision을 올린 새 ticket으로 복귀한다.

## Acceptance

- population 생성, 결과 draw와 rating 갱신은 같은 seed와 입력에서 동일하다.
- 한 경기의 양 팀 rating delta는 player 수가 같은 5v5에서 zero-sum이다.
- Flow가 실제 HTTP initial ticket ingestion, plan/reserve/confirm/acknowledgment와 revised ticket return을 통과한다.
- lifecycle operation 사이에도 due arrival이 유입되고, 두 개 이상의 game이 동시에 진행될 수 있다.
- 완료 party가 즉시 일괄 재큐잉되지 않고 0–30초 사이의 분산된 cooldown을 거친다.
- 모든 event에서 `idle + queued + in-game + cooldown = population`이 유지된다.
- 충분한 step 뒤 completed match, rating range 확대와 revision 2 이상의 returned ticket이 관측된다.
- TUI가 population의 idle/queue/in-game/cooldown player, completed game, rating range/distribution과 최근 승패를 표시한다.
- 기본 interactive workload는 1,000명이며 test fixture는 같은 contract의 축소 population을 사용할 수 있다.
- Unicode, ASCII, reduced-motion과 terminal-independent snapshot contract를 유지한다.

## Out Of Scope

- production-calibrated MMR, uncertainty 또는 confidence interval.
- party 생성·해체, 영구 이탈/신규 가입 churn과 실제 접속률 calibration.
- map, character, role별 performance와 draw/rematch/season reset.
- 경기 서버 simulation, 전투 replay 또는 external result ingestion.
- rating 보호, placement match, smurf detection과 anti-abuse policy.

## Completion Evidence

population core와 HTTP closed-loop focused test, TUI snapshot/width test, Go race gate와 `scripts/check.sh`를 통과한다. 모델의 수치와 한계는 `docs/sema-flow.md`가 소유한다.
