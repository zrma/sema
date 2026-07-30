# Wire Conformance

## Purpose

`cmd/sema-conformance`는 stable `/v1` service의 repository-owned reference client다. provider-neutral bearer token 세 개만 받아 health, authentication, authorization, tenant isolation과 ticket-to-assignment lifecycle을 실제 HTTP wire에서 검증한다. 실제 게임 session이나 caller-specific token acquisition은 실행하지 않는다.

검증 순서는 다음과 같다.

1. `/livez`와 `/readyz`.
2. token 없는 ticket read의 `401 Unauthenticated`.
3. same-tenant read-only token으로 ticket write할 때 `403 PermissionDenied`.
4. 네 match ticket 생성.
5. other-tenant read token으로 첫 ticket을 조회할 때 `404 NotFound`.
6. immutable policy 등록과 planning run.
7. proposal read, reservation create와 confirm.
8. assignment `completed` acknowledgment.

성공 report schema는 `sema.wire-conformance.v1`이다. report에는 random run ID와 각 경계의 boolean 판정만 포함하며 token, tenant, resource payload나 endpoint를 출력하지 않는다.

## Configuration

| Environment | Required permission |
|---|---|
| `SEMA_TARGET_BASE_URL` | HTTPS service base URL; `-base-url`로 대체 가능 |
| `SEMA_TARGET_WRITE_TOKEN` | 같은 tenant의 lifecycle read/write scope 전체 |
| `SEMA_TARGET_READ_TOKEN` | 같은 tenant의 `match_tickets.read` |
| `SEMA_TARGET_OTHER_TENANT_TOKEN` | 다른 tenant의 `match_tickets.read` |

세 token은 서로 달라야 하며 environment로만 전달한다. client는 redirect를 따르지 않고 response body를 1 MiB로 제한하며 오류에 token이나 response payload를 그대로 출력하지 않는다. HTTPS가 기본이고 `-allow-http`는 repository-owned isolated local fixture에서만 사용한다.

```sh
export SEMA_TARGET_BASE_URL='https://<target-service>'
export SEMA_TARGET_WRITE_TOKEN='<same-tenant-full-lifecycle-token>'
export SEMA_TARGET_READ_TOKEN='<same-tenant-match-ticket-read-token>'
export SEMA_TARGET_OTHER_TENANT_TOKEN='<other-tenant-match-ticket-read-token>'

sema-conformance
```

`cmd/sema-target-smoke`와 `sema.target-smoke.v1` report는 P30 `/v0alpha2` command/report compatibility를 위해 같은 구현 위에 보존한다. 신규 automation은 `/v1`을 기본으로 하는 `sema-conformance`와 `sema.wire-conformance.v1`을 사용한다. tagged predecessor 검사처럼 alpha route가 필요한 경우에만 `sema-conformance -api-version v0alpha2`를 명시한다.

## Repository-Owned Fixture

`scripts/check-postgres.sh`는 disposable PostgreSQL schema, ephemeral TLS OIDC discovery/JWKS provider와 표준 `sema-service` listener를 조립한다. issuer가 서명한 full-lifecycle, same-tenant read-only와 other-tenant read-only token을 `internal/wireconformance`에 주입하고 위 전체 순서를 실행한다.

```sh
scripts/check-postgres.sh
```

이 fixture는 current source revision의 wire semantics, OIDC claim mapping과 PostgreSQL authority가 함께 동작함을 검증한다. 별도 HTTPS reverse-proxy fixture는 external TLS termination 뒤의 private HTTP listener를 legacy compatibility client로 실행한다. 특정 provider나 private deployment inventory에 의존하지 않는다.

P30 legacy client의 고정 baseline과 stable cross-version requirement는 `docs/wire-compatibility.md`가 소유한다. `v0.3.0`과 `v0.4.0`의 tagged matrix가 첫 immutable alpha evidence이며 stable successor도 supported predecessor/current 양방향 검사를 반복한다.

`cmd/sema-wire-fixture`는 tagged client-service matrix가 각 immutable source에서 빌드하는 loopback-only test service다. current source에서는 `/v1`과 `/v0alpha2` target handler가 같은 in-memory repository를 사용한다. 세 test token을 환경으로 받지만 PostgreSQL, OIDC, TLS 또는 deployment readiness를 주장하지 않는다. 해당 운영 경계는 계속 `scripts/check-postgres.sh`와 container/recovery gate가 소유한다. fixture는 non-loopback listen을 거부하고 release artifact에는 포함하지 않는다.
