# Observability Contract

표준 PostgreSQL/OIDC runtime `cmd/sema-service`와 V0 development/reference runtime `cmd/sema-server`는 같은 bounded-cardinality HTTP metric과 redacted request trace contract를 사용한다. raw URL, tenant와 resource identity는 telemetry에 넣지 않는다.

표준 runtime의 `/metrics`, `/livez`와 `/readyz`는 private listener의 운영 endpoint다. external gateway는 lifecycle API만 허용하고 이 endpoint를 public route로 노출하지 않는다.

## Operational Endpoints

- `GET /livez`: HTTP process가 요청을 처리하는지 확인한다.
- `GET /readyz`: 표준 runtime은 PostgreSQL connectivity, V0는 durable runtime usable state를 확인한다.
- `GET /metrics`: Prometheus text exposition을 반환한다.
- V0 only `GET /v0alpha1/audit?after=<sequence>&limit=<1..1000>`: redacted durable audit page를 반환한다.

health는 runtime별 JSON envelope, metrics는 Prometheus text format이다. 표준 runtime의 durable audit authority는 PostgreSQL repository가 소유하며 V0 HTTP audit page를 그대로 노출하지 않는다.

## Metrics

`internal/observability.Recorder`가 다음 metric을 process-local로 집계한다.

- `sema_http_requests_total{method,route,status,code}`.
- `sema_http_request_duration_seconds{method,route,status,code}` histogram.

duration bucket은 1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s와 5s다. `route`는 실제 URL이 아니라 Go route pattern이므로 ticket/session/assignment ID를 label에 넣지 않는다. `code`는 bounded service failure code다.

현재 metric registry는 process restart 때 초기화된다. durable counters나 external Prometheus retention을 소유하지 않는다. `sema-standard-postgres-v1` workload는 lifecycle 뒤 `/metrics`를 실제 scrape해 주요 `v0alpha2` route pattern이 존재하고 run/resource identity가 없는지 검증한다.

## HTTP Trace

server는 W3C `traceparent`의 version 00 trace ID와 parent span ID를 받아 request span을 만든다. invalid/zero context는 새 trace로 대체하고 response에 새 `traceparent`를 반환한다.

`cmd/sema-service`와 `cmd/sema-server`는 span을 stderr JSON Lines로 출력한다.

```json
{"timestamp":"...","trace_id":"...","span_id":"...","method":"POST","route":"POST /v0alpha1/plans","status":200,"duration_ms":1.2}
```

span에는 query, raw URL, body, token, tenant, resource ID, journal payload와 error detail을 넣지 않는다. route pattern, status와 bounded failure code만 기록한다. 현재 OTLP exporter나 downstream span은 없다.

## Reference Alerts

`deploy/prometheus-rules.yaml`은 표준 runtime용 provider-neutral `PrometheusRule` 예제다.

- readiness `503` 지속: PostgreSQL authority reachability를 조사한다.
- `ResourceExhausted` 지속: request burst, caller retry와 replica별 admission/pool profile을 함께 본다.
- `Unavailable` 또는 `AuthenticationUnavailable` 지속: PostgreSQL 또는 OIDC/JWKS dependency를 구분한다.
- `v0alpha2` request p95가 750ms를 10분간 초과: `sema-standard-postgres-v1` reference regression budget과 비교한다.

이 rule은 alert backend, receiver, notification ownership, retention 또는 production SLA를 설치하지 않는다. deployment는 scrape target과 namespace/job scoping을 추가하고 실제 workload evidence로 threshold를 보정한다. tenant/resource label을 추가해 alert를 분할하지 않는다.

## V0 Decision Audit Export

raw journal은 replay source of truth라 player/ticket/session identity를 포함한다. HTTP exporter는 raw payload를 반환하지 않는다.

각 audit summary는 sequence, event kind, record checksum과 event-specific bounded summary만 포함한다.

- ticket ingestion: player count.
- backfill ingestion: team/open-slot count.
- plan: proposal/unmatched count와 budget flag.
- reservation: ticket count와 backfill flag.
- confirm/cancel/ack: status outcome.

checksum은 record integrity 대조용이고 authenticity proof가 아니다. raw journal access와 retention은 service operator 권한에 남는다.

## Privacy And Cardinality Gate

fixture는 trace, metrics와 V0 JSON audit을 직렬화한 뒤 known token, tenant, ticket/player/snapshot/reservation identity가 나타나지 않는지 검증한다. 새 label, span field 또는 audit attribute는 bounded cardinality와 tracked-artifact privacy를 함께 검토해야 한다.

## Verification

```sh
go test ./internal/observability ./internal/httpapi ./internal/targetruntime ./internal/serviceworkload
go test -race ./internal/observability ./internal/httpapi ./internal/targetruntime ./internal/serviceworkload
scripts/check-postgres.sh
```
