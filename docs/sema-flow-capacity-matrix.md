# Sema Flow Capacity Matrix

## Purpose

`cmd/sema-flow-matrix`는 같은 simulated duration, population과 timing에 여러 concurrent/batch profile을 적용하고 여러 deterministic seed의 결과를 min/median/max로 집계한다. 각 조합은 독립 Flow runtime이며 wall-clock worker parallelism은 report 순서와 값에 영향을 주지 않는다.

기본 matrix는 10분, seed `42,73,101`과 concurrent/batch `8/2,16/4,32/8`이다.

```sh
go run ./cmd/sema-flow-matrix
go run ./cmd/sema-flow-matrix -format json > flow-matrix.json
```

축소 비교는 profile을 `concurrent:batch` 형식으로 지정한다.

```sh
go run ./cmd/sema-flow-matrix \
  -duration 3m \
  -population 100 \
  -seeds 42,73,101 \
  -profiles 4:2,8:4 \
  -parallel 2
```

## Contract

- JSON schema는 `sema.flow.capacity-matrix.v0alpha1`이다.
- 모든 range text는 `minimum/median/maximum` 순서다.
- seed와 profile은 숫자 기준 canonical order로 정렬하며 중복을 거부한다.
- 짝수 sample의 integer median은 가운데 두 값의 midpoint를 내림한 값이다.
- `demand_comparable=true`는 같은 seed의 모든 profile이 같은 initial ticket 수를 관측하고 모든 run의 maximum arrival lag와 final ingress backlog가 0일 때만 성립한다.
- report는 run 수, assignment yield, confirmed/completed throughput, wait p50/p90, queue mean/p95와 skill-gap p90을 profile별로 집계한다.
- player/ticket identity, raw journal payload와 wall-clock duration은 포함하지 않는다.

## Reference Result

P15 기본 3x3 matrix는 모든 run에서 initial ticket 600개, maximum arrival lag 0ms와 final ingress backlog 0을 기록해 `demand_comparable=true`를 만족했다.

| Profile | Assignment yield | Confirmed throughput | Wait p50 | Wait p90 | Queue mean | Queue p95 | Skill gap p90 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 8 / 2 | 5,238 / 5,252 / 5,254 bps | 10,000 / 10,000 / 10,000 milli-match/min | 116,000 / 117,000 / 118,000 ms | 232,000 / 234,000 / 235,000 ms | 399 / 401 / 401 | 837 / 841 / 851 | 16 / 16 / 16 |
| 16 / 4 | 7,009 / 7,014 / 7,040 bps | 18,700 / 18,700 / 18,800 milli-match/min | 64,000 / 64,000 / 65,000 ms | 135,000 / 136,000 / 140,000 ms | 317 / 317 / 318 | 729 / 741 / 745 | 22 / 23 / 25 |
| 32 / 8 | 8,539 / 8,549 / 8,562 bps | 32,700 / 32,700 / 32,800 milli-match/min | 26,000 / 26,000 / 26,000 ms | 59,000 / 60,000 / 61,000 ms | 190 / 190 / 190 | 524 / 532 / 533 | 25 / 26 / 27 |

이 synthetic workload에서는 동시 match 수가 늘수록 throughput과 assignment yield가 올라가고 wait/queue pressure가 내려간다. 동시에 skill-gap p90은 8/2보다 16/4와 32/8에서 높다. target wait, 허용 quality loss, server cost나 traffic calibration이 없으므로 이 결과만으로 한 profile을 권장하지 않는다.

## Truth Boundary

- 세 seed의 min/median/max는 deterministic sensitivity range이며 confidence interval이 아니다.
- fixed 1,000-player closed population은 actual producer arrival, permanent churn이나 party reformation을 나타내지 않는다.
- concurrent match 수는 Flow simulation slot이지 game-server allocation cost나 infrastructure saturation 측정값이 아니다.
- profile admission에는 별도의 wait/quality target, cost model과 실제 traffic calibration이 필요하다.
- production throughput, maximum wait SLA나 scheduler capacity를 이 matrix로 선언하지 않는다.
