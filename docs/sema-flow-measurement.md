# Sema Flow Measurement

## Purpose

`cmd/sema-flow-report`는 P12 closed-loop Flow를 고정된 simulated duration 동안 headless로 실행하고 queue wait, assignment yield, match throughput, queue saturation과 proposal quality를 aggregate한다. TUI presentation 속도나 wall-clock 실행 시간은 report에 영향을 주지 않는다.

기본 실행은 seed 42, player 1,000명과 P12의 5v5 timing/policy를 30 simulated minutes 동안 측정한다.

```sh
go run ./cmd/sema-flow-report
go run ./cmd/sema-flow-report -format json > flow-report.json
```

축소 fixture나 다른 capacity를 비교할 때는 report에 기록되는 configuration을 함께 바꾼다.

```sh
go run ./cmd/sema-flow-report \
  -duration 10m \
  -population 100 \
  -matches-per-cycle 2 \
  -concurrent-matches 8
```

## Metric Contract

- `assignment_yield_basis_points`: 측정 구간에 queue로 들어온 player entry 중 같은 구간 안에 assignment가 confirm된 비율. 반복 복귀는 새 entry이며 구간 끝에 대기 중인 entry는 right-censored 상태로 분모에 남는다.
- `wait`: confirm된 각 ticket의 `confirmed_at - enqueued_at`을 party player 수만큼 가중한 nearest-rank p50/p90/p99/max.
- `confirmed_matches_per_minute_milli`, `completed_matches_per_minute_milli`: simulated minute당 match 수의 1/1000 fixed-point 값. `9366`은 분당 9.366 match다.
- `queue.mean_players`: event 사이 simulated duration으로 가중한 평균 queued player 수.
- `queue.p95_players`: 같은 duration weight에 nearest-rank를 적용한 queue occupancy.
- saturation basis points: queued player 수를 고정 population으로 나눈 비율.
- `quality.team_skill_gap`, `quality.max_latency_millis`: confirm된 proposal evidence의 nearest-rank 분포.
- `final`: 측정 종료 시 population state와 visible rating 분포. `idle + queued + in_game + cooldown = population_players`를 유지한다.

JSON schema는 `sema.flow.measurement.v0alpha1`이다. report는 seed, simulated duration과 workload configuration을 포함하며 ticket/player ID, raw durable payload, local path, host 정보와 wall-clock benchmark 결과는 포함하지 않는다.

## Reference Baseline

P13의 기본 30분 run은 다음 aggregate를 만든다. 이 값은 regression 비교를 위한 synthetic reference이며 capacity나 제품 SLA가 아니다.

| Metric | Value |
| --- | ---: |
| assignment yield | 7,578 bps |
| wait p50 / p90 / p99 | 432,000 / 575,000 / 584,000 ms |
| confirmed / completed throughput | 9,366 / 9,133 milli-match/min |
| queue mean / p95 / peak | 734 / 907 / 914 players |
| skill gap p50 / p90 / p99 / max | 7 / 23 / 45 / 58 |

초기 party는 첫 10분에 걸쳐 유입된다. 기본 동시 match 8개와 45초 game capacity보다 closed population의 재진입 수요가 크므로 이후 queue wait와 saturation이 상승한다. 이 현상은 reference workload의 capacity pressure이며 production traffic claim이 아니다.

## Determinism And Interpretation

due arrival은 예약된 server-clock 시각에 처리하며 자체적으로 simulated time을 추가 소비하지 않는다. plan, reserve, confirm과 completion operation만 고정된 lifecycle duration을 전진시키고, idle 구간은 tick duration만큼 전진한다. 따라서 TUI에서 한 arrival을 한 frame으로 보여줘도 request rendering이 queue wait를 늘리지 않는다.

같은 configuration의 JSON은 deterministic해야 한다. metric 변경을 비교할 때는 schema, seed, duration, population과 configuration을 모두 같게 두고 aggregate field를 비교한다.

## Truth Boundary

- report는 synthetic closed population과 reference planner policy를 측정하며 arbitrary external producer traffic을 관측하지 않는다.
- queue wait와 throughput은 Flow의 single-process serialized HTTP lifecycle을 포함하며 production scheduler나 multi-replica service capacity가 아니다.
- hidden true skill은 game result에만 사용되고 quality skill gap은 planner가 본 visible rating evidence다.
- multi-seed confidence interval, actual concurrency calibration, churn, rating uncertainty와 policy optimization은 포함하지 않는다.
- 이 report만으로 matchmaking cycle SLO, maximum wait SLA나 production capacity를 선언하지 않는다.
