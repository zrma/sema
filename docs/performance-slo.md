# Reference Performance SLO

## Claim Boundary

`sema-reference-container-v1`은 repository release regression을 판정하는 target profile이다. 특정 게임의 production SLA, internet-facing response time, queue wait SLO 또는 production storage guarantee가 아니다. 실제 consumer workload와 hardware가 정해지면 별도 profile과 decision으로 calibration한다.

## Target Profile

- pinned Go 1.26 Linux builder와 `scratch` runtime image.
- native Linux architecture, maximum 2 CPU와 2 GiB memory.
- service run은 network disabled, non-root/read-only runtime과 local Docker named volume을 사용한다.
- workload는 2x5 solo, cycle당 20 ticket, concurrency 8, 10 cycle이다.
- service workload를 새 volume에서 3회 연속 실행한다.
- Go benchmark는 같은 pinned builder에서 최소 3회 실행하고 각 metric의 최악값을 사용한다.

CPU model과 underlying storage는 runner마다 다를 수 있다. 따라서 budget은 미세 최적화가 아니라 order-of-magnitude regression을 차단할 headroom을 둔다.

## Service SLO

각 run은 lifecycle count, metric scrape, complete assignment restart replay와 torn-tail recovery를 모두 통과해야 한다.

| Metric | Budget |
|---|---:|
| measured HTTP operation p95 | 250 ms |
| single measured HTTP operation max | 1,000 ms |
| 10-cycle validator duration | 30,000 ms |
| consecutive passing runs | 3 |

HTTP operation에는 policy/ticket mutation, plan, reserve, confirm과 acknowledgment가 포함된다. audit page와 metric scrape는 latency sample에서 제외되지만 correctness gate에는 포함된다.

## Go Benchmark Budget

| Benchmark | ns/op max | B/op max | allocs/op max |
|---|---:|---:|---:|
| planner 50v50 solo | 5,000,000 | 1,000,000 | 1,500 |
| planner 100K queue, window 256 | 200,000,000 | 60,000,000 | 120,000 |
| engine 1000-ticket lifecycle | 20,000,000 | 2,000,000 | 5,000 |
| durable 1002-event replay | 200,000,000 | 5,000,000 | 40,000 |

`internal/performance`는 Go benchmark text에서 위 네 leaf benchmark만 읽고 GOMAXPROCS suffix와 host header를 버린다. 각 sample set의 최대값이 세 budget을 모두 만족해야 한다.

## History Artifact

CI performance job은 다음 sanitized file을 30일 artifact로 보존한다.

- `profile.json`: resource/workload/budget identity.
- `go-benchmarks.json`: sample count, 최악값, budget과 pass flag.
- `service-run-01.json`부터 `service-run-03.json`: aggregate lifecycle/latency/recovery report.

raw `go test -bench` stdout에는 runner CPU 정보가 포함될 수 있으므로 temp file에서만 사용하고 artifact와 tracked source에 넣지 않는다. service report에도 resource ID, journal path와 machine identity가 없다.

## Gate

```sh
scripts/check-performance.sh
```

report를 보존하려면 새 output directory를 지정한다.

```sh
scripts/check-performance.sh <report-directory>
```

budget 변경은 workload/profile, repeated evidence와 rationale을 ADR 및 이 문서에 함께 반영한다. 실패한 수치를 숨기기 위해 sample 수를 줄이거나 threshold만 높이지 않는다.
