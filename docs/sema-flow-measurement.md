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
- `ingress.samples_tickets`: 구간 안에 scheduled arrival timestamp대로 queue에 등록된 initial/return ticket 수.
- `ingress.max_arrival_lag_millis`: 처리된 ticket의 `event_at - scheduled_at` 최댓값. reference fixture에서는 0이어야 한다.
- `ingress.final_backlog_*`: 측정 종료 timestamp까지 도착했지만 아직 queue 등록 event로 처리되지 않은 ticket/player 수. horizon의 due event를 drain한 뒤 0이어야 한다.
- `quality.team_skill_gap`, `quality.max_latency_millis`: confirm된 proposal evidence의 nearest-rank 분포.
- `final`: 측정 종료 시 population state와 visible rating 분포. `idle + ingress_backlog + queued + in_game + cooldown = population_players`를 유지한다.

JSON schema는 `sema.flow.measurement.v0alpha2`다. report는 seed, simulated duration과 workload configuration을 포함하며 ticket/player ID, raw durable payload, local path, host 정보와 wall-clock benchmark 결과는 포함하지 않는다.

## Reference Baseline

P14의 기본 30분 run은 다음 aggregate를 만든다. 이 값은 regression 비교를 위한 synthetic reference이며 capacity나 제품 SLA가 아니다.

| Metric | Value |
| --- | ---: |
| assignment yield | 7,705 bps |
| wait p50 / p90 / p99 | 416,000 / 530,000 / 536,000 ms |
| confirmed / completed throughput | 10,133 / 9,933 milli-match/min |
| queue mean / p95 / peak | 731 / 909 / 920 players |
| ingress samples / max lag / final backlog | 2,374 tickets / 0 ms / 0 players |
| skill gap p50 / p90 / p99 / max | 9 / 28 / 39 / 55 |

초기 party는 첫 10분에 걸쳐 유입된다. 기본 동시 match 8개와 45초 game capacity보다 closed population의 재진입 수요가 크므로 이후 queue wait와 saturation이 상승한다. 이 현상은 reference workload의 capacity pressure이며 production traffic claim이 아니다.

같은 10분 demand horizon의 seed 42 single-run comparison은 scheduler가 capacity 설정과 무관하게 initial party 600개를 모두 ingress하는지 확인하는 P14 gate다.

| Concurrent / batch | Initial tickets | Max arrival lag | Final ingress backlog | Confirmed throughput | Wait p50 | Queue mean |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 8 / 2 | 600 | 0 ms | 0 players | 10,000 milli-match/min | 118,000 ms | 401 players |
| 16 / 4 | 600 | 0 ms | 0 players | 18,800 milli-match/min | 64,000 ms | 317 players |
| 32 / 8 | 600 | 0 ms | 0 players | 32,800 milli-match/min | 26,000 ms | 190 players |

이 표는 동일 수요 분모를 검증하는 deterministic fixture다. 한 seed의 synthetic 결과이므로 confidence interval이나 production capacity frontier가 아니다.

## Determinism And Interpretation

clock은 다음 scheduled arrival, reservation/confirmation stage, game completion, planning eligibility 또는 bounded tick 중 가장 이른 timestamp로만 전진한다. due arrival은 같은 timestamp의 기존 lifecycle transition보다 먼저 처리하고 한 batch의 reservation과 confirmation은 stage별로 같은 timestamp를 공유하며 proposal ID 순서로 방출한다. event를 TUI에서 한 frame씩 보여줘도 같은 timestamp의 frame 수가 queue wait를 늘리지 않는다.

measurement는 종료 timestamp에 이미 due인 event를 모두 drain한다. 아직 timestamp가 오지 않은 return은 cooldown이고, timestamp가 지났지만 처리되지 않은 arrival만 ingress backlog다.

같은 configuration의 JSON은 deterministic해야 한다. metric 변경을 비교할 때는 schema, seed, duration, population과 configuration을 모두 같게 두고 aggregate field를 비교한다.

## Truth Boundary

- report는 synthetic closed population과 reference planner policy를 측정하며 arbitrary external producer traffic을 관측하지 않는다.
- queue wait와 throughput은 Flow의 single-process serialized HTTP lifecycle을 포함하며 production scheduler나 multi-replica service capacity가 아니다.
- hidden true skill은 game result에만 사용되고 quality skill gap은 planner가 본 visible rating evidence다.
- multi-seed confidence interval, actual concurrency calibration, churn, rating uncertainty와 policy optimization은 포함하지 않는다.
- 이 report만으로 matchmaking cycle SLO, maximum wait SLA나 production capacity를 선언하지 않는다.
