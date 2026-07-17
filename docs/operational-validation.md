# Operational Validation

## Purpose

`cmd/sema-ops-check`는 P9 service와 durable runtime을 하나의 격리된 운영 시나리오로 검증한다. 외부 endpoint나 사용자 journal을 받지 않고 command가 만든 임시 directory와 loopback HTTP server만 사용한다.

## Workload

각 cycle은 다음 순서로 실행된다.

1. 2 team x 5 player policy를 등록한다.
2. solo `MatchTicket`을 concurrent HTTP PUT으로 제출한다.
3. 하나의 snapshot을 plan하고 모든 authoritative proposal ID를 reserve한다.
4. 모든 reservation을 confirm하고 assignment를 `completed`로 acknowledge한다.
5. 마지막 cycle 뒤 redacted audit 전체를 page로 읽고 Prometheus request metric을 확인한다.

ticket 수는 10의 배수이고 cycle당 최대 250이다. report는 aggregate count, 전체 HTTP mutation latency의 p50/p95/p99/max, metric 확인 결과와 recovery flag만 포함한다. ticket, proposal, reservation, assignment ID와 임시 path는 JSON에 포함하지 않는다.

## Recovery Injection

부하가 끝나면 command는 HTTP server와 runtime을 정상 종료하고 같은 journal을 다시 연다. 완료된 모든 assignment와 audit 시작 sequence가 복구됐는지 확인한다. 이어서 newline 없는 불완전한 record를 journal 끝에 append하고 다시 시작해 complete checksummed prefix가 유지되고 incomplete tail이 제거되는지 확인한다.

complete record corruption refusal, append/sync failure rollback과 second-writer rejection은 `internal/durable`의 focused fixture가 소유한다. 이 command는 process restart와 disk write interruption을 end-to-end service 경로에 연결한다.

## Commands

빠른 저장소 gate와 같은 범위는 다음과 같다.

```sh
go run ./cmd/sema-ops-check -cycles 1 -tickets-per-cycle 20 -concurrency 4 -timeout 30s
```

장시간 soak는 동일한 deterministic workload의 cycle 수와 전체 timeout만 늘린다.

```sh
go run ./cmd/sema-ops-check -cycles 1000 -tickets-per-cycle 20 -concurrency 16 -timeout 30m
```

soak command는 매 실행마다 새 임시 journal을 만들고 종료 시 제거한다. 실제 deployment 성능이나 데이터 복구를 시험하는 명령이 아니며 외부 service URL을 받지 않는다.

## Interpretation

- non-zero exit: lifecycle, metrics, restart 또는 tail recovery contract가 깨졌다.
- `metrics_verified=true`: plan route의 bounded request counter가 scrape에 존재한다.
- `restart_verified=true`: terminal assignment와 audit prefix가 replay됐다.
- `torn_tail_recovered=true`: incomplete final write가 제거됐다.
- latency fields: 해당 실행의 관측값이며 hardware-independent product SLO가 아니다.

CI quick run은 correctness/recovery regression gate다. numeric SLO는 target runtime profile, repeated sample과 budget을 별도 milestone에서 확정한다.

## Verification

```sh
go test ./internal/operational ./cmd/sema-ops-check
go test -race ./internal/operational ./cmd/sema-ops-check
```
