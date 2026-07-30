# ADR 0028: Standard Service Runtime

- Status: Accepted
- Date: 2026-07-30

## Context

P30은 PostgreSQL repository, provider-neutral OIDC verifier와 authenticated `v0alpha2` API를 `cmd/sema-target-server`로 조립했다. 그러나 image와 Compose example의 기본 entrypoint는 여전히 unauthenticated single-writer journal인 `cmd/sema-server`였다. 신규 설치의 제품 표면과 reference/compatibility 표면이 서로 다른 authority를 가리키는 상태였다.

기존 명령을 제거하거나 제자리에서 의미를 바꾸면 alpha 사용자와 V0 import fixture를 불필요하게 깨뜨린다. 표준 runtime 승격과 과거 명령의 호환 보존을 분리해야 한다.

## Decision

- `cmd/sema-service`를 PostgreSQL/OIDC 기반 stateless service의 표준 command로 둔다.
- `cmd/sema-service`와 `cmd/sema-target-server`는 `internal/serviceapp`의 같은 구현을 호출한다. 후자는 P30 command compatibility를 위한 alias이며 새 deployment 문서의 기본값이 아니다.
- `cmd/sema-server`는 V0 journal development/reference 및 optional import compatibility command로 보존한다. 기존 flags, journal format과 import source를 변경하지 않는다.
- container의 기본 entrypoint는 `sema-service`다. V0 전용 volume 선언과 default journal command는 image 기본 계약에서 제거하되, image 안의 `sema-server` binary로 명시적으로 실행할 수 있다.
- `deploy/compose.yaml`은 외부에서 제공되는 PostgreSQL/OIDC configuration을 받아 migration runner를 먼저 실행하고 표준 service를 시작한다. tracked example에는 credential이나 provider-specific inventory를 넣지 않는다.
- 표준 service startup은 migration을 암묵적으로 실행하지 않고 external TLS termination을 계속 명시적으로 요구한다.

## Consequences

- README, image, Compose와 primary operations runbook이 같은 PostgreSQL/OIDC authority를 가리킨다.
- 신규 설치는 V0 journal volume을 만들지 않는다. V0 사용자는 compatibility command와 별도 runbook을 명시적으로 선택한다.
- 기존 `sema-target-server`와 `sema-server` 호출은 유지되므로 이번 승격은 command removal이나 wire migration이 아니다.
- target compatibility command의 제거 시점은 multi-version evidence와 별도 deprecation decision 이후다.

## Alternatives Rejected

- `cmd/sema-server`의 구현을 PostgreSQL runtime으로 교체: 같은 command가 기존 flags와 authority를 조용히 바꾼다.
- `cmd/sema-target-server`를 그대로 제품 명칭으로 유지: 임시 target이라는 milestone 용어가 장기 표준 surface에 남는다.
- V0 binary와 import path를 즉시 제거: optional V0 recovery fixture와 기존 alpha 사용자를 근거 없이 깨뜨린다.
