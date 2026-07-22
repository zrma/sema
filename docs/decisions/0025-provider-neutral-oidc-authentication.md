# ADR 0025: Provider-Neutral OIDC Authentication

- Status: Accepted
- Date: 2026-07-22

## Context

target API는 deployment-owned `Authenticator`가 `Principal`을 공급하도록 이미 분리되어 있지만 실제 remote listener에는 credential protocol과 executable verifier가 필요하다. 첫 deployment에서 특정 identity provider를 사용하더라도 provider의 SDK, custom endpoint, client-secret 형식이나 orchestration inventory를 Sema core에 넣으면 다른 conforming provider로 교체하기 어렵고 공개 저장소에 deployment detail이 새어 나온다.

Sema는 token issuer가 아니라 OAuth2 resource server다. 호출자의 browser login, `client_credentials`, workload federation과 secret rotation은 token acquisition boundary이며 request마다 target service가 수행할 일이 아니다.

## Decision

- `internal/authn/oidc`가 deployment-neutral `targetapi.Authenticator` 구현을 제공한다.
- startup은 configured issuer의 OIDC discovery document를 HTTPS로 읽고 exact issuer와 HTTPS JWKS URL을 요구한다.
- request는 정확히 하나의 `Authorization: Bearer <JWT>` header만 받는다. query, cookie와 alternate header credential은 지원하지 않는다.
- verifier는 signature, configured asymmetric signing algorithm allowlist, exact issuer, audience, expiry와 optional not-before를 검증한다. 기본 signing algorithm은 `RS256`이며 symmetric algorithm과 unsigned token은 구성할 수 없다.
- `sub`는 provider-opaque subject, 기본 custom claim `sema_tenant`는 정확히 하나의 tenant가 된다. tenant는 path, query 또는 body에서 보충하거나 override하지 않는다.
- standard string `scope` claim에서 Sema의 고정 permission vocabulary와 exact-match하는 값만 principal permission으로 올린다. unknown scope는 권한을 만들지 않는다.
- token acquisition credential과 client secret은 Sema configuration에 포함하지 않는다. remote runtime에는 issuer, audience와 optional tenant-claim/signing-algorithm 설정만 주입한다.
- discovery/JWKS network와 HTTP failure는 provider unavailable로 분리한다. malformed, forged, expired 또는 claim-invalid token은 unauthenticated다. cached key로 검증 가능한 token은 issuer의 일시 장애 중에도 계속 검증할 수 있다.
- provider I/O는 bounded HTTP timeout을 사용하고 JWKS는 unknown key ID에서 refresh한다.

## Consequences

- 실제 deployment는 conforming OIDC provider와 credential lifecycle을 자유롭게 선택할 수 있고 Sema core/service/repository는 바뀌지 않는다.
- identity provider의 custom scope/property mapping은 `sema_tenant`와 Sema permission scope를 발급하도록 deployment source가 소유한다.
- one-token/one-tenant contract가 명확해져 cross-tenant fan-out은 privileged claim이나 request field로 암묵적으로 추가할 수 없다.
- issuer가 unavailable해도 cached signing key로 검증되는 token은 service될 수 있다. 새 key가 필요한 동안에는 retryable authentication unavailable을 반환한다.
- revocation introspection을 request마다 수행하지 않는다. emergency revocation latency는 short-lived access-token lifetime, signing-key rotation과 gateway deny policy가 소유한다.
- provider-specific end-to-end fixture는 deployment acceptance이며 Sema의 default test에는 ephemeral TLS discovery/JWKS conformance server를 사용한다.

## Alternatives Rejected

- provider 전용 SDK/adapter를 core에 직접 import: token validation contract와 deployment product lifecycle을 결합한다.
- proxy가 보낸 unsigned identity header 신뢰: trusted-proxy/mTLS binding 없이 header spoofing 경계가 생긴다.
- opaque token introspection per request: identity provider availability와 latency를 모든 matchmaking request의 synchronous dependency로 만든다.
- shared static API key: tenant/permission/key rotation과 workload identity를 하나의 장기 secret에 결합한다.
- tenant를 request payload에서 받기: authenticated identity와 repository scope 사이의 authority를 분리한다.
