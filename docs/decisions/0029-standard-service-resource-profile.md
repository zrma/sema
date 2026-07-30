# ADR 0029: Standard Service Resource Profile

- Status: Accepted
- Date: 2026-07-30

## Context

표준 PostgreSQL/OIDC service는 bounded HTTP admission을 갖고 있었지만 PostgreSQL pool은 `pgxpool`의 CPU-dependent default를 사용했고 API operation deadline과 numeric target-service regression profile은 없었다. 기존 `sema-reference-container-v1`은 V0 journal service를 측정하므로 새 PostgreSQL runtime의 근거가 될 수 없다.

초기 concurrency 32 workload는 기존 repository transaction이 scope row에 `ON CONFLICT` lock을 잡은 뒤 `FOR UPDATE`로 승격하면서 lock-upgrade deadlock을 만드는 것을 발견했다. pool이나 timeout을 늘리는 것은 이 correctness 문제를 숨길 뿐이다.

## Decision

- scope mutation transaction은 가장 먼저 atomic upsert로 다음 tenant repository version을 예약한다. 이후 operation/resource lock은 같은 canonical order로 처리하며 rollback은 예약 version도 함께 되돌린다.
- service replica당 PostgreSQL pool default는 maximum 16, minimum idle 2 connection으로 고정한다. CPU 수에 따른 implicit pool sizing을 사용하지 않는다.
- target API default admission은 64 in-flight이고 operation deadline은 5초다. 초과 요청은 즉시 retryable `ResourceExhausted`를 반환한다.
- `sema-standard-postgres-v1`은 concurrency 32에서 100-ticket/10-match lifecycle 3 cycle을 독립적으로 3회 실행한다.
- reference budget은 run별 p95 750ms, single request 2초, run duration 30초이며 admission rejection과 canceled pool acquire는 0이어야 한다.
- cryptographic OIDC 성능은 이 profile에서 분리한다. provider-neutral OIDC correctness와 실제 service composition은 wire conformance가 계속 소유한다.

## Consequences

- same-tenant mutation은 ordered version을 위해 직렬화되지만 concurrent lock upgrade deadlock은 회귀 테스트로 차단된다.
- pool과 admission 사이에는 최대 4:1의 bounded request-to-connection 비율이 생기며 짧은 acquire wait는 허용하지만 canceled acquire는 허용하지 않는다.
- repository-owned 수치는 release regression budget이지 production SLA나 database sizing recommendation이 아니다.
- deployment가 admission/pool/timeout을 바꾸면 같은 workload shape 또는 deployment-owned representative profile로 capacity와 tail latency를 다시 검증해야 한다.

## Alternatives Rejected

- `pgxpool` default 유지: service replica의 connection budget이 host CPU에 따라 암묵적으로 바뀐다.
- 32개 이상의 connection을 기본값으로 사용: same-tenant ordered commit이 주 병목인 baseline에서 database connection 소비만 늘릴 근거가 없다.
- request timeout만 늘려 deadlock을 통과: lock cycle과 tail latency를 숨기고 recovery를 늦춘다.
- 기존 V0 250ms SLO를 그대로 적용: storage와 lifecycle shape가 다른 profile을 같은 수치로 오인한다.
