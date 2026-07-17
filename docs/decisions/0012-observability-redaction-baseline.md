# ADR 0012: Low-Cardinality Observability And Redacted Audit

- Status: Accepted

## Context

P9 service는 durable lifecycle을 제공하지만 request latency, failure rate, readiness와 decision history를 외부에서 확인할 수 없다. raw path와 journal payload를 그대로 exporter에 쓰면 resource ID와 player data가 공개되고 metric cardinality가 queue 크기에 비례한다.

## Decision

- HTTP metrics는 method, route pattern, status와 bounded failure code만 label로 사용한다.
- standard-library Prometheus text endpoint와 fixed duration bucket을 제공한다.
- W3C traceparent를 받아 request-level JSON span을 만들고 route pattern만 기록한다.
- liveness는 HTTP process, readiness는 durable runtime usable state로 구분한다.
- decision audit exporter는 raw record payload를 버리고 sequence/kind/checksum과 count/flag/outcome만 반환한다.
- operational output에 known resource identity가 없는지 fixture로 검증한다.

## Consequences

- 별도 telemetry dependency 없이 P10 load/failure validation을 측정할 수 있다.
- metrics는 process restart 때 초기화되고 traces는 stderr sink에 한정된다.
- raw journal과 exported audit의 용도가 명확히 분리된다.
- per-ticket debugging은 exported telemetry가 아니라 권한 있는 raw journal investigation이 필요하다.

## Revisit Triggers

- OTLP collector, distributed downstream span 또는 baggage propagation이 필요하다.
- production metric retention/alerting backend가 정해진다.
- operator가 event timestamp, pseudonymous correlation 또는 additional audit field를 요구한다.
- cardinality/volume budget이 measured telemetry SLO를 넘는다.
