# ADR 0018: Authenticated Target API Boundary

- Status: Accepted
- Date: 2026-07-18

## Context

PostgreSQL repository는 tenant-scoped resource CAS와 operation/audit receipt를 제공하지만, 기존 HTTP `v0alpha1`은 unauthenticated loopback journal prototype이다. 이를 그대로 PostgreSQL에 연결하면 tenant authority, historical idempotency retry와 pagination consistency가 transport마다 달라지고, client가 Proposal 같은 authority-owned resource를 쓸 위험이 있다.

실제 identity provider는 아직 선택하지 않았으므로 provider protocol을 wire contract로 고정하지 않으면서 authenticated target behavior를 실행 가능한 fixture로 먼저 검증해야 한다.

## Decision

- 새 target surface는 experimental `v0alpha2`로 분리하고 `v0alpha1` route semantics를 변경하지 않는다.
- deployment-owned `Authenticator`가 subject, tenant와 permission을 결정한다. tenant는 HTTP path, query 또는 body에서 받지 않는다.
- authentication과 authorization은 repository lookup 전에 완료하며 cross-tenant resource existence를 노출하지 않는다.
- 첫 vertical slice는 client-owned `MatchTicket` create/higher-revision replace, exact cancel, get와 list/poll이다. generic resource write와 client-authored Proposal/Reservation/Assignment는 허용하지 않는다.
- 모든 mutation은 tenant-scoped `Idempotency-Key`와 canonical command digest를 요구한다.
- repository는 committed operation receipt를 mutation-state 검증 전에 resolve할 수 있어야 한다. 따라서 오래된 retry도 후속 resource 변경과 무관하게 최초 commit result를 반환한다.
- list cursor는 HMAC으로 인증하고 tenant, resource kind, filter, order와 repository snapshot version에 묶는다. snapshot version이 바뀌면 page를 혼합하지 않고 `StaleSnapshot`으로 처음부터 재시도하게 한다.
- target handler는 실제 identity provider나 remote listener를 포함하지 않는다. PostgreSQL-backed isolated composition fixture로 storage부터 HTTP polling까지 검증한다.

## Consequences

- `internal/api/v0alpha2`가 target envelope와 match-ticket resource/page schema를 소유한다.
- `internal/targetapi`가 provider-neutral authentication/authorization, strict HTTP validation과 opaque cursor를 소유한다.
- `internal/service.MatchTickets`가 wire와 adapter에서 독립적인 revision, tombstone, canonical persistence와 idempotent command semantics를 소유한다.
- `repository.Repository.Replay`가 historical operation result lookup을 명시하고 memory, file prototype과 PostgreSQL adapter가 같은 conformance를 통과한다.
- PostgreSQL primary와 stateless replica 결정은 유지되며 Redis, broker와 outbox는 추가하지 않는다.
- 실제 provider, credential lifecycle, TLS, rate limit과 remote deployment는 다음 cutover decision이다.

## Verification

- unauthenticated/provider unavailable/permission-denied behavior를 분리한다.
- 두 tenant가 같은 resource/operation ID를 독립적으로 사용하고 다른 tenant resource를 읽지 못한다.
- create 뒤 higher revision을 저장한 후 최초 operation을 재시도해도 최초 version이 replay된다.
- cursor 변조, tenant/filter/order binding 위반과 stale snapshot을 거부한다.
- tombstone이 active get/list에서 제외되고 identity reuse를 거부한다.
- isolated PostgreSQL schema에서 authenticated create와 poll이 같은 target handler를 통과한다.

## Revisit Triggers

- identity provider와 tenant credential lifecycle이 확정된다.
- consumer가 snapshot restart 대신 long-lived consistent page나 streaming delivery를 요구한다.
- full-scope repository snapshot이 measured list/planning latency budget을 초과한다.
- permission model이 per-resource policy 또는 delegated tenant administration을 요구한다.
