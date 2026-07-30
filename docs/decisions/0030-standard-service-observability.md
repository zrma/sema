# ADR 0030: Standard Service Observability

## Status

Accepted

## Context

표준 PostgreSQL/OIDC runtime은 health endpoint와 workload budget을 갖췄지만 API route별 failure와 latency를 외부에서 판단할 contract가 없었다. raw path, token, tenant 또는 resource ID를 telemetry에 사용하면 private data 노출과 unbounded cardinality가 생긴다. 특정 monitoring vendor를 runtime에 포함하면 provider-neutral service boundary도 약해진다.

## Decision

- V0에서 검증한 in-process Prometheus recorder와 W3C `traceparent` redacted JSON span을 표준 runtime에도 재사용한다.
- metric label과 span route는 Go route pattern, method, status와 bounded failure code만 사용한다.
- 인증은 route match 뒤 실행해 인증 실패와 admission rejection도 실제 bounded endpoint pattern으로 기록한다.
- `/metrics`, `/livez`와 `/readyz`는 token 없는 private operational endpoint이며 public gateway route가 아니다.
- `cmd/sema-service`는 span을 stderr JSON Lines로 내보내고 telemetry backend 선택은 deployment에 남긴다.
- readiness, continuous admission exhaustion, dependency failure와 750ms reference p95를 `deploy/prometheus-rules.yaml`의 기준 alert로 제공한다.
- standard workload는 lifecycle 뒤 metrics scrape와 identity 비노출을 acceptance에 포함한다.

## Consequences

표준 runtime은 특정 observability backend 없이도 scrape와 trace 수집이 가능하고, alert rule은 repository workload budget과 연결된다. registry와 trace sink는 process-local이며 retention, receiver, job scoping과 production threshold calibration은 deployment 책임이다. tenant/resource별 troubleshooting은 metric label이 아니라 private application data와 repository authority를 사용해야 한다.

## Rejected Alternatives

- raw URL이나 tenant/resource ID를 label로 사용: private data와 cardinality가 queue/resource 수에 비례한다.
- `/metrics`를 bearer-authenticated lifecycle API로 노출: monitoring credential과 caller tenant authority를 불필요하게 결합한다.
- OTLP/Prometheus backend를 runtime에 내장: repository가 provider와 retention을 소유하게 된다.
- workload latency만 관측하고 runtime telemetry는 생략: 실제 dependency/admission failure를 운영 중 구분할 수 없다.

## Revisit When

- process-local metric volume이 measured memory budget을 넘는다.
- downstream trace correlation 또는 repository transaction span이 incident evidence에 필요하다.
- 실제 deployment profile이 reference alert threshold와 다른 sustained baseline을 입증한다.
