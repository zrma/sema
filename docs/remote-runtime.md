# Authenticated Remote Runtime

## Boundary

`cmd/sema-target-server`는 PostgreSQL repository, provider-neutral OIDC verifier와 authenticated `v0alpha2` target API를 하나의 stateless service process로 조립한다. 실제 identity provider 제품, token acquisition, TLS gateway, database provisioning과 secret delivery는 deployment 책임이다.

기존 `cmd/sema-server`는 V0 journal/reference runtime이며 target server의 alias가 아니다. target writer cutover 전후 rollback 계약을 섞지 않는다.

## Required Configuration

| Environment | Secret | Contract |
|---|---:|---|
| `SEMA_POSTGRES_DSN` | yes | migrated target PostgreSQL connection string |
| `SEMA_CURSOR_KEY_BASE64` | yes | base64로 인코딩한 최소 32-byte HMAC key |
| `SEMA_OIDC_ISSUER` | no | exact HTTPS discovery issuer |
| `SEMA_OIDC_AUDIENCE` | no | access-token audience |
| `SEMA_TLS_TERMINATION` | no | 현재는 명시적 `external`만 허용 |
| `SEMA_OIDC_TENANT_CLAIM` | no | optional; 기본 `sema_tenant` |
| `SEMA_OIDC_SIGNING_ALGORITHMS` | no | optional comma-separated asymmetric allowlist; 기본 `RS256` |

Sema에는 OIDC client ID/secret, token endpoint credential 또는 provider-specific SDK configuration을 넣지 않는다. caller가 어떤 방식으로 token을 얻는지는 별도 deployment contract다.

## Startup Order

1. target database/schema를 provisioning하고 credential을 private secret source에 저장한다.
2. 같은 image의 `sema-postgres-migrate`를 pre-traffic Job으로 실행한다.
3. OIDC issuer에 audience, tenant claim과 Sema permission scope를 발급하도록 deployment mapping을 구성한다.
4. external TLS gateway와 private-only Service/listener reachability를 구성한다.
5. target server를 시작하고 `/livez`, `/readyz`를 통과시킨다.
6. 실제 caller token으로 unauthenticated, permission-denied, tenant isolation과 allowed lifecycle을 검증한 뒤 writer traffic을 연다.

로컬 placeholder 형식은 다음과 같다. 실제 값은 tracked shell script나 문서에 저장하지 않는다.

```sh
export SEMA_POSTGRES_DSN='<postgres-dsn>'
export SEMA_CURSOR_KEY_BASE64='<base64-encoded-random-32-byte-or-longer-key>'
export SEMA_OIDC_ISSUER='https://<identity-provider>/application/o/sema/'
export SEMA_OIDC_AUDIENCE='sema'
export SEMA_TLS_TERMINATION='external'

go run ./cmd/sema-postgres-migrate
go run ./cmd/sema-target-server -listen 127.0.0.1:8080
```

## Listener And Health

- default listen은 `0.0.0.0:8080`이며 plaintext private application hop이다.
- external gateway는 client-facing TLS를 종료하고 pod listener는 public ingress에서 직접 접근할 수 없어야 한다.
- Sema는 `X-Forwarded-*` identity나 unsigned principal header를 신뢰하지 않는다. bearer token을 직접 검증한다.
- `GET /livez`는 process liveness, `GET /readyz`는 bounded PostgreSQL connectivity다. 두 endpoint는 token을 요구하지 않고 repository/provider detail을 반환하지 않는다.
- API는 기본 128 concurrent request까지만 admission하고 초과 요청을 retryable 503으로 반환한다. `-max-in-flight`는 1부터 4096까지이며 production 값은 workload evidence로 조정한다.
- HTTP server는 32 KiB header, bounded read/write/idle timeout을 사용하고 target handler는 request body를 1 MiB로 제한한다.

OIDC discovery/JWKS는 image의 public CA trust bundle을 사용한다. private CA가 필요하면 deployment가 trust bundle을 안전하게 주입해야 하며 TLS 검증을 끄는 option은 제공하지 않는다.

## Failure And Rotation

- missing/invalid environment, unsupported TLS mode, schema mismatch 또는 initial OIDC discovery failure는 listener open 전 process를 실패시킨다.
- cursor key rotation은 기존 pagination cursor를 무효화한다. rollout 동안 cursor continuity가 필요하면 모든 replica가 같은 active key를 사용해야 한다.
- OIDC signing-key rotation은 unknown key ID에서 JWKS를 refresh한다. cached key token은 provider의 일시 장애 중에도 검증할 수 있다.
- PostgreSQL readiness failure는 pod를 traffic에서 제외하지만 liveness는 유지해 transient database outage에 process restart loop를 만들지 않는다.
- service startup은 migration을 수행하지 않는다. migration Job 성공과 compatible binary rollout ordering은 deployment source of truth가 소유한다.

## Deployment Acceptance

- migration Job이 target schema version을 설치하고 재실행해도 idempotent하다.
- pod spec에는 OIDC client secret이 없고 PostgreSQL/cursor credential만 private secret reference에서 온다.
- public route는 TLS이며 application Service는 허용된 gateway/caller namespace 외에서 접근할 수 없다.
- no token은 401, valid token/insufficient scope는 403, provider key refresh outage는 503이다.
- token의 tenant claim을 바꾼 두 principal이 서로의 resource를 보거나 cursor를 재사용할 수 없다.
- two-replica lifecycle contention이 repository conformance와 같은 single authority 결과를 만든다.

위 acceptance가 실제 deployment에서 통과하기 전에는 remote runtime executable의 존재를 production cutover 완료로 해석하지 않는다.
