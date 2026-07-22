# OIDC Authentication Contract

## Scope

Sema target API는 OAuth2 resource server다. `internal/authn/oidc`는 conforming OpenID Connect issuer가 발급한 signed JWT access token을 검증하고 `targetapi.Principal`로 변환한다. identity provider 제품, 사용자 로그인, token endpoint 호출, client credential과 deployment route는 이 package의 책임이 아니다.

## Required Provider Contract

- issuer는 HTTPS OIDC discovery를 제공하고 discovery의 `issuer`가 configured value와 exact match한다.
- discovery는 HTTPS `jwks_uri`를 제공한다.
- access token은 configured asymmetric signing algorithm, issuer와 audience를 사용한다.
- token에는 non-empty `sub`, string `sema_tenant`, expiration과 optional string `scope`가 있다.
- access-token lifetime과 caller credential rotation은 deployment가 소유한다.

기본 claim 예시는 다음과 같다.

```json
{
  "iss": "https://<identity-provider>/application/o/sema/",
  "aud": "sema",
  "sub": "game-backend",
  "exp": 1784732400,
  "sema_tenant": "tenant-a",
  "scope": "match_tickets.write planning_runs.write assignments.read"
}
```

실제 issuer, subject, tenant와 timestamp는 tracked fixture나 로그에 보존하지 않는다.

## Permission Mapping

`scope`의 whitespace-delimited 값 중 다음 exact vocabulary만 권한으로 인식한다.

- `match_tickets.read`, `match_tickets.write`
- `backfill_tickets.read`, `backfill_tickets.write`
- `policies.read`, `policies.write`
- `planning_runs.read`, `planning_runs.write`
- `reservations.read`, `reservations.write`
- `assignments.read`, `assignments.write`

unknown scope는 무시한다. permission이 없는 valid principal은 인증에는 성공하지만 해당 endpoint에서 `PermissionDenied`다.

## Failure Contract

| Condition | Target API result |
|---|---|
| missing/multiple/malformed bearer header | `Unauthenticated` |
| malformed signature, wrong issuer/audience, expired/not-yet-valid token | `Unauthenticated` |
| missing/non-string tenant or non-string scope | `Unauthenticated` |
| valid identity without required permission | `PermissionDenied` |
| discovery/JWKS network or upstream HTTP failure | retryable `AuthenticationUnavailable` |

JWT, raw claim set와 credential은 error response, audit 또는 metrics label에 기록하지 않는다.

## Deployment Boundary

Sema remote runtime은 generic issuer, audience, tenant claim과 signing-algorithm configuration만 받는다. token을 발급받는 game/backend workload는 deployment-selected mechanism을 사용한다. 같은 cluster의 workload federation, 외부 service account, gateway TLS와 provider property mapping은 private deployment source of truth가 소유한다.

default repository test는 ephemeral HTTPS discovery/JWKS provider를 사용해 issuer/audience/time/claim validation, key rotation, provider outage와 target API 401/403/503 mapping을 실행한다. provider-specific acceptance는 같은 public contract를 실제 deployment에서 다시 검증한다.
