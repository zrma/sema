# Sema Flow Capacity Matrix

## Purpose

`cmd/sema-flow-matrix`는 같은 simulated duration, population과 timing에 여러 `matches_per_cycle` profile을 적용하고 여러 deterministic seed의 결과를 min/median/max로 집계한다. 각 조합은 독립 Flow runtime이며 wall-clock worker parallelism은 report 순서와 값에 영향을 주지 않는다.

기본 matrix는 10분, seed `42,73,101`과 batch `2,4,8`이다.

```sh
go run ./cmd/sema-flow-matrix
go run ./cmd/sema-flow-matrix -format json > flow-matrix.json
```

축소 비교는 cycle당 match 수를 쉼표로 지정한다.

```sh
go run ./cmd/sema-flow-matrix \
  -duration 3m \
  -population 100 \
  -seeds 42,73,101 \
  -batches 2,4 \
  -parallel 2
```

## Contract

- JSON schema는 `sema.flow.capacity-matrix.v0alpha2`다.
- 모든 range text는 `minimum/median/maximum` 순서다.
- seed와 profile은 숫자 기준 canonical order로 정렬하며 중복을 거부한다.
- 짝수 sample의 integer median은 가운데 두 값의 midpoint를 내림한 값이다.
- `demand_comparable=true`는 같은 seed의 모든 profile이 같은 initial ticket 수를 관측하고 모든 run의 maximum arrival lag와 final ingress backlog가 0일 때만 성립한다.
- report는 run 수, assignment yield, confirmed/completed throughput, wait p50/p90, queue mean/p95와 skill-gap p90을 profile별로 집계한다.
- player/ticket identity, raw journal payload와 wall-clock duration은 포함하지 않는다.

## Reference Result

P16 기본 3x3 matrix는 모든 run에서 initial ticket 600개, maximum arrival lag 0ms와 final ingress backlog 0을 기록해 `demand_comparable=true`를 만족했다.

| Profile | Assignment yield | Confirmed throughput | Wait p50 | Wait p90 | Queue mean | Queue p95 | Skill gap p90 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| b2 | 7,437 / 7,440 / 7,448 bps | 21,800 / 21,800 / 21,900 milli-match/min | 52,000 / 53,000 / 53,000 ms | 117,000 / 118,000 / 118,000 ms | 288 / 289 / 289 | 701 / 703 / 708 | 23 / 26 / 26 |
| b4 | 8,731 / 8,776 / 8,808 bps | 36,900 / 37,100 / 37,200 milli-match/min | 14,000 / 15,000 / 15,000 ms | 44,000 / 45,000 / 45,000 ms | 145 / 145 / 147 | 448 / 462 / 472 | 23 / 24 / 27 |
| b8 | 9,808 / 9,834 / 9,864 bps | 47,600 / 48,200 / 48,200 milli-match/min | 6,000 / 6,000 / 6,000 ms | 12,000 / 12,000 / 12,000 ms | 56 / 57 / 58 | 99 / 102 / 102 | 14 / 17 / 19 |

이 synthetic workload에서는 cycle당 match 수가 늘수록 throughput과 assignment yield가 올라가고 wait/queue pressure가 내려간다. skill-gap p90은 이 세 seed에서 단조 증가하지 않았다. target wait, 허용 quality loss나 traffic calibration이 없으므로 이 결과만으로 한 batch를 권장하지 않는다.

## Truth Boundary

- 세 seed의 min/median/max는 deterministic sensitivity range이며 confidence interval이 아니다.
- fixed 1,000-player closed population은 actual producer arrival, permanent churn이나 party reformation을 나타내지 않는다.
- 진행 중인 game 수는 Flow의 read model이며 profile이나 planning gate가 아니다.
- profile admission에는 별도의 wait/quality target과 실제 traffic calibration이 필요하다.
- production throughput, maximum wait SLA나 scheduler capacity를 이 matrix로 선언하지 않는다.
