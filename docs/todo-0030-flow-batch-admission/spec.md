# P19 Flow Batch Admission Spec

- Status: Completed

## Objective

Flow reference workload가 planning interval마다 고정된 두 match만 요청해 P18 multi-proposal planner의 출력을 인위적으로 제한하던 admission 병목을 제거한다. `matches_per_cycle`은 채워야 할 고정 batch가 아니라 한 planning cycle에서 반환할 proposal의 상한으로 사용한다.

## Contract

- 기본 planning interval은 5초를 유지해 해당 구간에 모인 queue snapshot을 한 번에 탐색한다.
- queue에 5v5 한 match를 만들 10명 이상이 있으면 configured upper bound를 채우지 못해도 planning을 실행한다.
- 기본 `matches_per_cycle` 상한은 32이고 configuration safety bound는 256이다.
- backlog가 충분하면 한 cycle에서 수 개부터 수십 개의 disjoint proposal을 반환할 수 있다.
- reservation과 confirmation은 반환된 proposal 전체에 이어지고 TUI `MATCH LIFECYCLE` 관찰 surface를 유지한다.
- active game 수와 frontend-owned game execution은 planning admission을 제한하지 않는다.
- TUI header는 최근 선택 수, configured upper bound와 planning interval을 함께 표시한다.

## Acceptance

- 32-match 상한에서도 한 match 분량만 있으면 partial batch가 즉시 반환된다.
- 400-player backlog fixture가 한 cycle에서 32개의 proposal을 반환한다.
- 1,000-player closed-loop 30분 run에서 queue wait가 선형 증가하지 않고 ingress lag/backlog가 0이다.
- 10분 warm-up 뒤 20분 정상상태 구간이 분당 80개 이상의 confirmed match를 처리한다.
- Flow TUI/report/matrix 기본값과 도움말이 upper-bound semantics를 사용한다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- 32는 Flow synthetic workload의 실용적인 기본 burst 상한이지 production capacity나 제품 SLA가 아니다.
- 256은 configuration 폭주를 막는 safety bound이며 planner algorithm의 이론적 한계가 아니다.
- 정상상태 처리량은 closed population과 synthetic return schedule에서 얻은 회귀 evidence다. arbitrary external producer traffic이나 multi-replica service capacity를 나타내지 않는다.

## Completion Evidence

- backlog regression에서 두 번째 planning cycle이 32개의 mutually disjoint proposal을 반환했다.
- seed 42, 1,000-player 30분 run은 confirmed 2,295 match, 누적 76.5 match/min, wait p50/p90/p99 5/9/18초, maximum 28초와 final queue 91 players를 기록했다.
- 같은 run의 10분 시점 497 match를 제외한 이후 20분에는 1,798 match, 즉 89.9 match/min이 confirm되었다.
- 10분 run은 wait p50/p90/p99 5/7/10초, maximum 18초와 ingress lag/backlog 0을 기록했다.

