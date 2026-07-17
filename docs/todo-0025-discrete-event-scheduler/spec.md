# P14 Discrete-Event Scheduling Baseline Spec

- Status: Completed

## Objective

Flow의 presentation step과 simulated server clock을 분리하고 arrival, plan, reservation, confirmation과 completion을 logical timestamp 순서로 실행한다. 높은 batch/concurrency에서도 due arrival이 lifecycle event에 밀리지 않아 capacity report가 동일한 demand horizon을 비교하게 한다.

## Problem Evidence

P13의 10분 comparison에서 `32 concurrent / batch 8`은 최초 600 party 중 458개만 ingress했고 종료 시 522 player가 return schedule에 남았다. 같은 horizon의 낮은 concurrency fixture보다 queue가 작고 wait가 짧게 보였지만 이는 demand가 queue 밖에 지연된 결과다. presentation용 serialized step이 workload admission을 바꾸면 capacity 비교의 분모가 달라진다.

## Scheduling Contract

- initial arrival은 empty start 다음 `arrival_interval`부터 시작하며 마지막 party도 자신의 scheduled timestamp를 소유한다.
- clock은 다음 arrival, scheduled lifecycle operation, game completion, planning eligibility 또는 bounded tick 중 가장 이른 timestamp까지만 전진한다.
- due arrival은 같은 timestamp의 다른 lifecycle transition보다 먼저 ingress한다.
- 한 proposal batch의 reservation stage와 confirmation stage는 각각 같은 logical timestamp를 공유하고 stable proposal ID 순서로 event를 방출한다.
- 같은 timestamp의 event는 TUI에서 여러 frame으로 보일 수 있지만 frame 수가 simulated time을 추가하지 않는다.
- future return만 cooldown이며 scheduled timestamp가 지난 initial/return arrival은 ingress backlog로 별도 집계한다.

## Acceptance

- arrival, pending operation과 completion의 next timestamp가 deterministic하다.
- 같은 timestamp의 tie는 event kind와 stable resource ID로 재현 가능하게 정렬된다.
- population은 `idle + ingress backlog + queued + in-game + cooldown = population`을 유지한다.
- processed arrival의 `event_at - scheduled_at`은 모든 reference fixture에서 0이다.
- measurement horizon 끝에 due event를 모두 drain하고 final ingress backlog는 0이다.
- 10분 8/16/32 concurrent comparison 모두 scheduled initial party 600개를 ingress한다.
- `sema.flow.measurement.v0alpha2` report가 ingress sample, maximum arrival lag와 final backlog를 노출한다.
- Unicode/ASCII/reduced-motion TUI와 deterministic snapshot contract를 유지한다.
- focused, race와 full repository gate를 통과한다.

## Out Of Scope

- 실제 HTTP request latency나 multi-worker concurrency model.
- production ingress rate, scheduler SLO 또는 product queue SLA.
- multi-seed capacity frontier와 automatic configuration selection.
- 외부 event broker, shared clock 또는 distributed simulation.

## Completion Evidence

- event ordering/clock fixture가 empty start, due arrival 우선순위와 same-stage stable proposal ordering을 고정한다.
- high-batch horizon regression이 scheduled initial party 전체, maximum arrival lag 0ms와 final backlog 0을 검증한다.
- 10분 seed 42 comparison의 8/16/32 concurrent fixture가 모두 initial party 600개, maximum arrival lag 0ms와 final backlog 0을 기록했다.
- `sema.flow.measurement.v0alpha2`, TUI `ready`/`cooldown` 구분, Unicode/ASCII/reduced-motion snapshot과 repository gate를 검증한다.

P15 capacity matrix는 이 동일-demand contract 위에서 multi-seed 결과와 변동성을 비교한다.
