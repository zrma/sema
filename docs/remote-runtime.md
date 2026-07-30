# Authenticated Remote Runtime

## Boundary

`cmd/sema-service`는 PostgreSQL repository, provider-neutral OIDC verifier와 authenticated `v0alpha2` target API를 하나의 stateless service process로 조립하는 표준 command다. 실제 identity provider 제품, token acquisition, TLS gateway, database provisioning과 secret delivery는 deployment 책임이다.

P30에서 사용한 `cmd/sema-target-server`는 같은 구현을 호출하는 command compatibility alias다. 기존 `cmd/sema-server`는 V0 journal development/reference 및 optional import compatibility runtime이며 PostgreSQL service의 alias가 아니다. 두 runtime의 authority와 recovery 계약을 섞지 않는다.

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
2. 같은 image의 `sema-postgres-migrate`를 service startup 전 migration Job으로 실행한다.
3. OIDC issuer에 audience, tenant claim과 Sema permission scope를 발급하도록 deployment mapping을 구성한다.
4. external TLS gateway와 private-only Service/listener reachability를 구성한다.
5. `sema-service`를 시작하고 `/livez`, `/readyz`를 통과시킨다.
6. 전용 acceptance caller token으로 unauthenticated, permission-denied, tenant isolation과 allowed lifecycle을 검증한다.

로컬 placeholder 형식은 다음과 같다. 실제 값은 tracked shell script나 문서에 저장하지 않는다.

```sh
export SEMA_POSTGRES_DSN='<postgres-dsn>'
export SEMA_CURSOR_KEY_BASE64='<base64-encoded-random-32-byte-or-longer-key>'
export SEMA_OIDC_ISSUER='https://<identity-provider>/application/o/sema/'
export SEMA_OIDC_AUDIENCE='sema'
export SEMA_TLS_TERMINATION='external'

go run ./cmd/sema-postgres-migrate
go run ./cmd/sema-service -listen 127.0.0.1:8080
```

## Listener And Health

- default listen은 `0.0.0.0:8080`이며 plaintext private application hop이다.
- external gateway는 client-facing TLS를 종료하고 pod listener는 public ingress에서 직접 접근할 수 없어야 한다.
- Sema는 `X-Forwarded-*` identity나 unsigned principal header를 신뢰하지 않는다. bearer token을 직접 검증한다.
- `GET /livez`는 process liveness, `GET /readyz`는 bounded PostgreSQL connectivity다. 두 endpoint는 token을 요구하지 않고 repository/provider detail을 반환하지 않는다.
- API는 기본 64 concurrent request까지만 admission하고 초과 요청을 retryable `ResourceExhausted` 503으로 반환한다. `-max-in-flight`는 1부터 4096까지이며 production 값은 workload evidence로 조정한다.
- API operation deadline은 기본 5초다. PostgreSQL pool은 replica당 maximum 16, minimum idle 2 connection을 사용하며 모든 값은 explicit flag로 조정할 수 있다.
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

repository-owned two-replica contention, restart와 PostgreSQL connection outage/recovery evidence는 `docs/runtime-failure-matrix.md`가 소유한다.
repository-owned pool/admission/operation deadline과 numeric regression budget은 `docs/service-workload.md`가 소유한다.

`sema-conformance`는 provider에서 발급한 token으로 health, no-token `401`, 같은
tenant read-only token의 write `403`, 다른 tenant의 resource non-disclosure와
policy/planning/reservation/assignment completion을 한 번에 검증한다. token은 flag나 tracked
파일이 아니라 process environment로만 전달한다.

```sh
export SEMA_TARGET_BASE_URL='https://<target-service>'
export SEMA_TARGET_WRITE_TOKEN='<same-tenant-full-lifecycle-token>'
export SEMA_TARGET_READ_TOKEN='<same-tenant-match-ticket-read-token>'
export SEMA_TARGET_OTHER_TENANT_TOKEN='<other-tenant-match-ticket-read-token>'

sema-conformance
```

세 token은 서로 달라야 한다. write token에는 이 문서의 lifecycle endpoint에 필요한 read와
write scope가 모두 있어야 하고, read token에는 `match_tickets.read`만, other-tenant token에는
다른 `sema_tenant`와 `match_tickets.read`가 있어야 한다. smoke run은 unique ID의 durable
acceptance resource를 남기므로 일반 caller tenant가 아니라 전용 E2E tenant/database에서
실행한다. provider outage `503`과 JWKS rotation은 token-only smoke로 만들지 않고 통제된
deployment failure drill에서 별도로 확인한다.

P30의 `sema-target-smoke` command와 `sema.target-smoke.v1` report는 compatibility alias로
보존한다. repository-owned provider-neutral PostgreSQL/OIDC fixture와 표준 report 계약은
`docs/wire-conformance.md`가 소유한다.

위 acceptance는 provider-neutral runtime을 실제 환경에서 조립할 수 있다는 evidence다. 실제 game traffic, existing deployment 또는 product adoption을 증명하지 않는다.
