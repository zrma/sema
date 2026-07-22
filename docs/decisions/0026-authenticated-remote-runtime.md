# ADR 0026: Authenticated Remote Runtime

- Status: Accepted
- Date: 2026-07-22

## Context

PostgreSQL repository, authenticated target API와 provider-neutral OIDC verifier가 각각 존재해도 이들을 실제 listener로 조립하는 executable이 없으면 remote service acceptance를 실행할 수 없다. 반대로 기존 `cmd/sema-server`를 제자리에서 바꾸면 V0 journal reference/rollback surface와 target writer가 암묵적으로 전환된다.

Sema는 외부 TLS gateway 뒤의 private application listener로 배포하기로 했다. application process가 gateway나 cluster network policy를 스스로 증명할 수는 없지만 TLS 소유자가 명시되지 않은 상태로 remote listener를 여는 것도 허용하면 안 된다.

## Decision

- `cmd/sema-target-server`를 PostgreSQL-backed authenticated `v0alpha2` 전용 executable로 추가하고 기존 V0 `cmd/sema-server`와 분리한다.
- startup은 PostgreSQL schema/version check와 OIDC discovery를 완료하고 handler 조립에 성공한 뒤에만 listener를 연다.
- PostgreSQL DSN과 최소 32-byte cursor HMAC key는 environment에서만 읽고 error 또는 startup log에 값을 출력하지 않는다.
- OIDC configuration은 issuer, audience, optional tenant claim과 asymmetric signing-algorithm allowlist만 받는다. token acquisition client secret은 받지 않는다.
- `SEMA_TLS_TERMINATION=external`을 명시해야 시작한다. 이는 TLS 구현을 가장하는 값이 아니라 deployment가 gateway/load balancer termination과 private listener reachability를 함께 검증해야 한다는 startup contract다.
- `/livez`와 `/readyz`만 unauthenticated다. readiness는 tenant scope를 읽거나 만들지 않고 PostgreSQL connectivity를 bounded timeout으로 확인한다.
- target request는 fixed maximum in-flight semaphore, HTTP header/read/write/idle timeout과 기존 1 MiB body limit을 적용한다. capacity를 넘으면 retryable `ResourceExhausted`다.
- schema migration은 service startup이 암묵적으로 실행하지 않는다. `cmd/sema-postgres-migrate`가 같은 DSN으로 명시적 pre-traffic migration을 실행한다.
- image에는 target server, migration runner, healthcheck와 HTTPS OIDC용 CA trust bundle을 포함한다. 기본 entrypoint는 cutover 승인 전까지 V0 server로 유지한다.

## Consequences

- provider-neutral binary 하나를 conforming OIDC issuer와 PostgreSQL deployment에 재사용할 수 있다.
- pod가 직접 public plaintext listener로 노출되지 않는다는 보장은 deployment NetworkPolicy, Service/Gateway route와 acceptance test가 소유한다.
- migration failure, schema mismatch, PostgreSQL/OIDC startup failure는 listener open 전 실패한다.
- issuer가 startup 뒤 일시적으로 unavailable해도 cached key로 검증되는 token은 계속 처리할 수 있고 새 key refresh 실패는 request 단위 503이 된다.
- health endpoint는 credential이나 tenant data를 노출하지 않으며 OIDC provider에 probe마다 의존하지 않는다.
- max in-flight 기본값은 안전한 시작점일 뿐 production quota/SLO가 아니다. workload evidence 뒤 조정한다.

## Alternatives Rejected

- 기존 V0 server를 target runtime으로 교체: rollback/reference surface와 writer cutover를 하나의 binary behavior change로 결합한다.
- startup automatic migration: 여러 replica rollout과 schema ownership 순서를 application startup에 숨긴다.
- application에서 provider token을 직접 발급받기: resource server와 caller credential lifecycle을 결합한다.
- unauthenticated health 외 별도 trusted header path: gateway identity binding 없이 인증 우회 경로가 된다.
- TLS mode 기본값을 `external`로 가정: deployment가 transport owner를 정하지 않은 상태도 정상 구성처럼 보이게 한다.
