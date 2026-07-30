# Operations Runbook

## Supported Deployment Envelope

표준 runtime은 `cmd/sema-service`가 제공하는 provider-neutral OIDC authenticated `v0alpha2` API다. PostgreSQL primary가 durable mutation authority이고 service replica는 stateless하다. Redis, broker, 실제 게임 session 실행과 token 발급은 Sema runtime의 baseline이 아니다.

client-facing TLS는 승인된 gateway/load balancer가 종료하고 Sema listener는 private network에서만 접근 가능해야 한다. process는 `SEMA_TLS_TERMINATION=external`이 없으면 시작하지 않으며 bearer token을 직접 검증한다. provider 제품별 설정, endpoint, credential과 private deployment inventory는 deployment-owned다.

## Container Contract

- default entrypoint: `/usr/local/bin/sema-service`.
- runtime identity: numeric non-root `65532:65532`.
- root filesystem: read-only 가능; `/tmp`만 bounded tmpfs로 제공한다.
- durable state: container volume이 아니라 `SEMA_POSTGRES_DSN`의 migrated PostgreSQL schema.
- capabilities: none; privilege escalation disabled.
- health: `/livez`는 process, `/readyz`는 bounded PostgreSQL connectivity.
- shutdown: `SIGTERM` 뒤 최대 10초 graceful HTTP drain; deployment stop grace는 15초 이상.

image에는 explicit migration runner인 `sema-postgres-migrate`, V0 compatibility command인 `sema-server`, P30 command compatibility를 위한 `sema-target-server`도 포함한다. 이 둘은 default entrypoint가 아니다.

## Required Configuration

다음 값은 runtime secret/configuration source에서 주입한다. 실제 값은 tracked file이나 command history에 남기지 않는다.

| Environment | Secret | Purpose |
|---|---:|---|
| `SEMA_POSTGRES_DSN` | yes | migrated PostgreSQL connection string |
| `SEMA_CURSOR_KEY_BASE64` | yes | 최소 32-byte cursor HMAC key의 base64 표현 |
| `SEMA_OIDC_ISSUER` | no | exact HTTPS discovery issuer |
| `SEMA_OIDC_AUDIENCE` | no | expected access-token audience |
| `SEMA_TLS_TERMINATION` | no | `external` |
| `SEMA_OIDC_TENANT_CLAIM` | no | optional; 기본 `sema_tenant` |
| `SEMA_OIDC_SIGNING_ALGORITHMS` | no | optional asymmetric allowlist; 기본 `RS256` |

Sema resource server에는 OIDC client secret이나 token acquisition credential을 주입하지 않는다. claim와 permission contract는 `docs/oidc-authentication.md`, startup security boundary는 `docs/remote-runtime.md`가 소유한다.

## Deployment Example

`deploy/compose.yaml`은 external PostgreSQL과 OIDC provider를 요구하는 local reference composition이다. migration service가 성공한 뒤 표준 service가 시작되고 host port는 `127.0.0.1:8080`에만 bind된다.

```sh
export SEMA_POSTGRES_DSN='<postgres-dsn>'
export SEMA_CURSOR_KEY_BASE64='<base64-encoded-random-32-byte-or-longer-key>'
export SEMA_OIDC_ISSUER='https://<identity-provider>/<issuer-path>/'
export SEMA_OIDC_AUDIENCE='sema'

docker compose -f deploy/compose.yaml up --build -d
docker compose -f deploy/compose.yaml ps
```

`migrate`가 성공하지 않거나 OIDC discovery가 완료되지 않으면 service listener는 열리지 않는다. local example의 loopback publish는 client-facing TLS를 제공하지 않으므로 실제 remote route의 대체물이 아니다.

종료는 다음과 같다.

```sh
docker compose -f deploy/compose.yaml stop
docker compose -f deploy/compose.yaml down
```

표준 service는 local journal volume을 만들지 않는다. PostgreSQL 데이터 제거·backup·restore는 database 운영 경계에서 별도로 승인하고 실행한다.

## Startup And Readiness

1. target PostgreSQL database/schema와 least-privilege credential을 준비한다.
2. service binary와 같은 revision의 `sema-postgres-migrate`를 one-shot Job으로 실행한다.
3. OIDC issuer에 audience, tenant claim과 Sema permission scope mapping을 구성한다.
4. external TLS gateway와 private listener reachability를 구성한다.
5. `sema-service`를 시작하고 `/livez`, `/readyz`를 확인한다.
6. repository-owned acceptance caller로 no-token 401, insufficient-scope 403, tenant isolation과 allowed lifecycle을 검증한다.

service startup은 schema migration을 암묵적으로 실행하지 않는다. 여러 replica는 같은 schema revision, cursor key, issuer/audience와 reservation TTL을 사용해야 한다.

## Failure Triage

| Symptom | Expected action |
|---|---|
| startup 전에 PostgreSQL failure | listener가 열리지 않은 상태를 유지하고 DSN reachability/schema migration을 확인한다 |
| startup OIDC discovery failure | issuer/TLS trust와 discovery document를 확인하고 credential을 log에 출력하지 않는다 |
| `/readyz` 503 | replica를 traffic에서 제외하고 PostgreSQL connectivity를 조사한다; liveness restart loop를 만들지 않는다 |
| authentication refresh 503 | provider/JWKS reachability를 조사한다; cached-key token과 신규 key token을 구분한다 |
| duplicate demand/reservation conflict | operation ID를 유지해 retry하고 PostgreSQL receipt/claim authority로 수렴시킨다 |
| cursor rejection after rollout | 모든 replica의 cursor key와 repository version을 확인한다 |

request, token, DSN, tenant resource와 raw database/log output은 private application data로 취급한다. 공개 issue나 tracked report에는 sanitized aggregate와 redacted 판정만 남긴다.

## Upgrade And Recovery

binary rollout 전에 migration Job을 실행하고 compatible schema가 확인된 뒤 stateless replica를 점진적으로 교체한다. rollback binary가 현재 schema와 wire contract를 읽을 수 있는지 확인하지 않은 상태에서 image만 되돌리지 않는다.

PostgreSQL backup encryption, retention, PITR, restore location과 RPO/RTO는 deployment 책임이다. 현재 repository gate는 disposable PostgreSQL의 logical backup/restore와 semantic manifest equality를 검증하지만 제품 backup/PITR acceptance는 P31의 남은 항목이다. 따라서 이 문서는 아직 특정 numeric recovery promise를 선언하지 않는다.

optional V0 journal import/recovery와 single-writer compatibility runtime은 `docs/v0-import.md`와 `docs/v0-runtime.md`를 따른다. 신규 설치는 V0 journal을 요구하지 않는다.

## Validation

```sh
go test -race ./internal/serviceapp ./internal/wireconformance ./internal/targetruntime ./internal/authn/oidc
scripts/check-postgres.sh
scripts/check-container.sh
```

첫 command는 command composition과 bounded runtime을, PostgreSQL gate는 repository/OIDC lifecycle, two-replica failure matrix 및 logical recovery fixture를, container gate는 standard entrypoint와 호환 binary/restart surface를 확인한다. 실제 provider reference deployment acceptance는 tracked credential 없이 `docs/remote-runtime.md`의 redacted acceptance 절차를 따른다.
