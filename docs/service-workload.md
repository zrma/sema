# Standard Service Workload

## Claim Boundary

`sema-standard-postgres-v1`은 표준 `v0alpha2` target handler와 실제 PostgreSQL repository의 release regression profile이다. production SLA, 실제 player arrival model, internet latency, OIDC provider 성능 또는 database 제품 sizing을 주장하지 않는다.

workload는 유효한 bearer token을 요구하는 authenticated handler 경로를 사용하지만 cryptographic OIDC verification은 deterministic valid-principal authenticator로 대체한다. OIDC discovery/JWKS/signature와 실제 `sema-service` 조립은 별도의 wire conformance가 검증한다. 이 분리로 database/service 변화와 외부 identity provider 변동을 같은 latency 수치에 섞지 않는다.

## Reference Profile

- disposable PostgreSQL 안에 run마다 새 schema를 만들고 migration한 뒤 종료 시 삭제한다.
- service replica 하나의 PostgreSQL pool은 maximum 16, minimum idle 2 connection이다.
- target API admission은 64 in-flight, operation deadline은 5초다.
- run마다 3 cycle, cycle마다 100 solo ticket을 concurrency 32로 ingest한다.
- 각 cycle은 5v5 match 10개를 plan, reserve, confirm, terminal acknowledge한다.
- 독립 run 3회를 연속 실행하므로 총 900 ticket, 90 match와 90 terminal assignment를 검증한다.
- policy/ticket/planning/proposal/reservation/assignment HTTP operation의 client-side latency와 PostgreSQL pool aggregate를 기록한다.
- lifecycle 뒤 `/metrics`를 별도 scrape해 bounded `v0alpha2` route와 private run identity 비노출을 검증한다.

이 profile에서 허용하는 regression budget은 다음과 같다.

| Metric | Budget |
|---|---:|
| request p95 per run | 750 ms |
| single request max | 2,000 ms |
| one 300-ticket run duration | 30,000 ms |
| `ResourceExhausted` responses below admission | 0 |
| canceled PostgreSQL pool acquire | 0 |
| consecutive independent runs | 3 |

초기 disposable PostgreSQL calibration에서 한 run은 3.5~5.9초, 5.1~8.6 match/s, request p95 283~506ms, maximum 476~900ms 범위였다. 750ms p95는 관측 최악값에 약 48% headroom을 둔 회귀선이다. hardware와 container runtime이 고정된 production 수치가 아니므로 실제 deployment는 자체 profile로 재측정한다.

## Runtime Defaults

`sema-service`의 repository-owned defaults는 다음과 같다.

- `-max-in-flight 64`
- `-request-timeout 5s`
- `-postgres-max-conns 16`
- `-postgres-min-idle-conns 2`
- `-postgres-max-conn-age 30m`
- `-postgres-max-conn-idle 5m`
- `-postgres-health-check-period 1m`

같은 tenant의 commit은 ordered repository version을 위해 직렬화하지만 transaction은 scope version을 가장 먼저 원자적으로 예약한다. resource lock을 잡은 뒤 scope lock을 승격하지 않으므로 concurrent request가 PostgreSQL lock-upgrade deadlock을 만들지 않는다. 서로 다른 tenant는 scope lock을 공유하지 않는다.

admission 초과는 대기하지 않고 `503`, `Retry-After: 1`, `X-Sema-Error-Code: ResourceExhausted`와 retryable error envelope를 반환한다. operation deadline 또는 dependency failure는 `Unavailable`이며 client는 두 결과를 구분해야 한다.

## Report

`sema-service-workload`는 `sema.service-workload.v1` JSON을 출력한다. report에는 profile/budget, run별 aggregate lifecycle count, metrics verification, latency, throughput과 pool count/wait만 들어간다. DSN, schema, token, tenant/resource ID, query와 machine identity는 포함하지 않는다.

CI PostgreSQL job은 `runtime-failure-matrix.json`과 `service-workload.json`을 30일 artifact로 보존한다.

test 전용 database에서 직접 실행한다.

```sh
SEMA_POSTGRES_TEST_DSN='<test-dsn>' \
  go run ./cmd/sema-service-workload \
  -runs 3 -cycles 3 -tickets-per-cycle 100 -concurrency 32
```

이 command는 격리 schema를 만들고 삭제하므로 shared 또는 production database를 대상으로 실행하지 않는다. 전체 PostgreSQL gate에도 같은 profile이 포함된다.

```sh
scripts/check-postgres.sh
```

budget 변경은 workload shape, 반복 관측값과 rationale을 이 문서와 ADR에 함께 남긴다. 실패를 숨기기 위해 run 수를 줄이거나 OIDC conformance를 이 profile 결과로 대체하지 않는다.
