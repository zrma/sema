# Target API Boundary

## Status

`v0alpha2`는 PostgreSQL target repository 위에서 검증하는 authenticated service boundary다. 현재 구현은 immutable `Policy` catalog, `MatchTicket`/`BackfillTicket` ingestion, repository-versioned planning run/result와 reservation/assignment lifecycle을 연결하며 stable compatibility나 production listener를 제공하지 않는다. 기존 `v0alpha1` journal service와 route semantics를 조용히 바꾸지 않는다.

## Authentication And Tenant Scope

`internal/targetapi.Authenticator`는 deployment-owned identity provider adapter가 구현한다. 성공 결과는 다음 값을 포함한다.

- bounded opaque subject
- 하나의 tenant scope
- `match_tickets.read`, `match_tickets.write` permission
- `backfill_tickets.read`, `backfill_tickets.write` permission
- `policies.read`, `policies.write` permission
- `planning_runs.read`, `planning_runs.write` permission
- `reservations.read`, `reservations.write` permission
- `assignments.read`, `assignments.write` permission

HTTP path, query와 body에는 tenant field가 없다. handler는 인증과 permission 확인을 repository lookup보다 먼저 수행하고, repository key의 scope는 항상 authenticated principal에서만 만든다. credential 부재/거부는 `Unauthenticated`, provider 장애는 retryable `AuthenticationUnavailable`, permission 부족은 `PermissionDenied`다.

실제 token protocol, issuer, tenant credential lifecycle과 TLS termination은 아직 선택하지 않았다. 따라서 target handler를 사용하는 remote executable도 아직 제공하지 않는다.

## Policy Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `PUT` | `/v0alpha2/policies/{version}` | `policies.write` | immutable version-to-content registration |
| `GET` | `/v0alpha2/policies/{version}` | `policies.read` | registered policy poll |
| `GET` | `/v0alpha2/policies` | `policies.read` | registered policy page |

policy version은 canonical rule content의 fingerprint에 묶인다. 같은 content 재등록은 별도 operation receipt를 남기지만 payload content는 바꾸지 않으며, 같은 version에 다른 content를 등록하면 `PolicyConflict`다. role requirement 순서는 canonicalize하고 relaxation 순서는 policy semantics로 보존한다. repository payload는 `sema.policy.v1` schema와 검증된 fingerprint를 함께 저장하며 read 때 content로 fingerprint를 다시 계산한다.

## Match Ticket Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `PUT` | `/v0alpha2/match-tickets/{ticket_id}` | `match_tickets.write` | create 또는 higher-revision replace |
| `GET` | `/v0alpha2/match-tickets/{ticket_id}` | `match_tickets.read` | active resource poll |
| `GET` | `/v0alpha2/match-tickets` | `match_tickets.read` | active resource page |
| `DELETE` | `/v0alpha2/match-tickets/{ticket_id}?revision=<exact>` | `match_tickets.write` | exact-revision tombstone |

모든 mutation은 정확히 하나의 `Idempotency-Key` header를 요구한다. 같은 tenant에서 같은 key와 canonical command digest를 재시도하면 후속 revision 변경이나 cancellation이 이미 일어났더라도 최초 commit version을 반환한다. 같은 key를 다른 command에 사용하면 `IdempotencyConflict`다.

repository payload는 wire envelope와 별개인 `sema.match-ticket.v1` schema로 저장한다. domain revision은 producer freshness이고 `storage_version`은 repository CAS/commit version이므로 서로 대체하지 않는다. tombstone identity는 새 ticket으로 재사용할 수 없으며 active list와 get에서는 보이지 않는다.

## Backfill Ticket Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `PUT` | `/v0alpha2/backfill-tickets/{ticket_id}` | `backfill_tickets.write` | create 또는 higher-revision/monotonic-roster replace |
| `GET` | `/v0alpha2/backfill-tickets/{ticket_id}` | `backfill_tickets.read` | active resource poll |
| `GET` | `/v0alpha2/backfill-tickets` | `backfill_tickets.read` | active resource page |
| `DELETE` | `/v0alpha2/backfill-tickets/{ticket_id}?revision=<exact>&roster_version=<exact>` | `backfill_tickets.write` | exact freshness tombstone |

replacement는 session identity를 바꾸지 않고 ticket revision을 전진시켜야 한다. roster version은 뒤로 갈 수 없으며 같은 roster version에서 vacancy shape나 existing roster aggregate를 변경할 수 없다. exact cancellation은 BackfillTicket tombstone과 session claim 해제를 한 transaction에서 기록한다.

`demand_identity` claim이 MatchTicket과 BackfillTicket의 tenant-scoped ID 중복을 차단하고 `backfill_session_claim`이 session마다 하나의 active demand만 허용한다. 이 claim은 repository-internal authority이며 client가 직접 쓰거나 list할 수 없다. ticket identity claim은 cancellation 뒤에도 남고 session claim만 해제되어 새 ticket ID가 같은 session을 다시 요청할 수 있다.

## Planning Run Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `POST` | `/v0alpha2/planning-runs/{run_id}` | `planning_runs.write` | policy version으로 immutable snapshot capture, matcher 실행과 result completion |
| `GET` | `/v0alpha2/planning-runs/{run_id}` | `planning_runs.read` | `planning` 또는 `completed` 상태 poll |
| `GET` | `/v0alpha2/planning-runs/{run_id}/proposals` | `planning_runs.read` | immutable proposal page |
| `GET` | `/v0alpha2/planning-runs/{run_id}/unmatched` | `planning_runs.read` | immutable unmatched ticket page |

POST body는 `policy_version`만 받는다. capture commit은 policy와 active demand의 exact repository version을 `sema.planning-snapshot.v1` payload로 저장하고 transaction을 닫는다. matcher는 저장된 snapshot으로 transaction 밖에서 실행한다. completion은 `planning_run`, 모든 `proposal`과 `planning_unmatched` resource를 한 atomic commit에 기록한다. capture 뒤 matcher가 중단되면 같은 `Idempotency-Key` retry가 현재 queue를 다시 읽지 않고 저장된 snapshot부터 재개한다.

planning command는 현재 synchronous HTTP response로 completion까지 기다리지만 `GET`은 concurrent execution 중 `planning` 상태도 읽을 수 있다. proposal은 planner만 생성하며 client가 placement body를 write할 endpoint는 없다. run ID는 immutable하고 다른 operation으로 재사용할 수 없다.

## Reservation Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `POST` | `/v0alpha2/reservations/{reservation_id}` | `reservations.write` | authoritative `proposal_id`의 current demand를 atomic claim |
| `GET` | `/v0alpha2/reservations/{reservation_id}` | `reservations.read` | active 또는 terminal reservation poll |
| `GET` | `/v0alpha2/reservations` | `reservations.read` | active와 terminal reservation page |
| `POST` | `/v0alpha2/reservations/{reservation_id}/cancel` | `reservations.write` | active reservation과 모든 demand claim을 atomic cancel |
| `POST` | `/v0alpha2/reservations/{reservation_id}/confirm` | `reservations.write` | current freshness 재검증, demand 소비와 pending assignment atomic create |

reserve body는 `proposal_id`만 받고 placement, ticket list와 expiry를 client가 제출할 수 없다. service는 저장된 proposal을 읽어 현재 ticket revision과 optional backfill session/roster version을 다시 검증하고, reservation과 ticket별 `demand_reservation_claim`을 한 commit으로 만든다. 같은 demand를 경쟁하는 replica 중 claim CAS winner만 성공한다.

TTL은 handler composition의 필수 양수 옵션이며 expiry는 server clock이 결정한다. command/get/list가 만료를 발견하면 reservation을 `expired`로 전환하고 claim 전체를 영속적으로 해제한다. cancel/expiry 뒤 같은 proposal은 다른 reservation이 다시 claim할 수 있지만 terminal reservation ID는 재사용하지 않는다.

reserve와 cancel은 최초 응답을 immutable internal `operation_result`에 저장한다. 따라서 뒤에 cancel/expiry가 일어난 후 최초 operation을 retry해도 current aggregate가 아니라 당시 status와 storage version을 반환한다. claim과 operation result는 client write/list surface에 노출하지 않는다.

confirm body는 `assignment_id`만 받는다. service는 reservation이 참조한 authoritative proposal placement와 current ticket/backfill freshness를 다시 확인한다. 성공하면 ticket/backfill tombstone, backfill session claim과 reservation claim 해제, reservation confirmed 전환, pending assignment와 operation result를 한 commit에 기록한다. freshness가 깨지면 assignment를 만들지 않고 reservation을 cancel해 claim을 해제하며 `StaleSnapshot`을 반환한다.

## Assignment Operations

| Method | Path | Permission | Semantics |
|---|---|---|---|
| `GET` | `/v0alpha2/assignments/{assignment_id}` | `assignments.read` | pending 또는 terminal assignment poll |
| `GET` | `/v0alpha2/assignments` | `assignments.read` | pending과 terminal assignment page |
| `POST` | `/v0alpha2/assignments/{assignment_id}/acknowledgments` | `assignments.write` | external application outcome의 idempotent terminal 기록 |

acknowledgment body는 `completed`, `cancelled` 또는 `failed` outcome과 outcome별 detail만 받는다. `Idempotency-Key`가 operation ID이며 response의 acknowledgment에 기록된다. 서로 다른 terminal outcome 경쟁은 assignment storage version CAS로 하나만 commit한다. 같은 operation retry는 original result를 반환하고 다른 terminal transition은 `InvalidTransition`이다.

backfill completion과 stale failure는 assignment target의 session ID/expected roster version을 제시하고 resulting roster version을 전진시켜야 한다. acknowledgment는 consumer가 외부 game/session에 적용한 결과를 기록할 뿐 Sema가 그 외부 상태를 변경하지 않는다. confirm 뒤 acknowledgment가 terminal로 바뀌어도 original confirm retry는 historical pending 응답을 반환한다.

## Pagination And Polling

list order는 `resource_id.asc`이고 기본 limit은 50, 최대는 200이다. `next_cursor`는 HMAC으로 인증한 opaque token이며 다음 context에 묶인다.

- authenticated tenant
- resource kind와 active filter
- stable order
- repository snapshot version
- 마지막 resource ID

cursor payload를 client state authority로 사용하지 않는다. 변조하거나 다른 tenant/filter/order에 재사용하면 `InvalidInput`이고, page 사이 tenant repository version이 바뀌면 retryable `StaleSnapshot`을 반환한다. consumer는 첫 page부터 다시 읽는다. 이 conservative fence는 중복/누락 없는 snapshot page를 우선하며, kind-specific database pagination 최적화는 측정된 snapshot 비용이 trigger가 될 때 추가한다.

Policy, active demand, reservation과 assignment page는 위 repository snapshot fence를 사용한다. completed planning result는 immutable하므로 proposal/unmatched cursor를 completed run의 storage version과 run ID에 묶는다. unrelated queue ingress가 일어나도 이미 시작한 result page는 stale해지지 않는다.

첫 delivery contract는 assignment와 ticket resource의 HTTP polling이다. stream, broker와 outbox는 baseline에 없다.

## Composition And Security Boundary

target handler는 `repository.Repository`, `Authenticator`, server clock, 명시적 reservation TTL과 최소 32-byte cursor authentication key를 주입받는다. PostgreSQL integration fixture가 migration된 isolated schema에서 Policy, 두 demand kind, planning result와 reservation/assignment full lifecycle을 실제 transaction으로 수행한다. cursor key와 database credential은 tracked configuration이나 response에 기록하지 않는다.

strict JSON decoding, 1 MiB body limit, bounded identifier와 allowlisted query parameter를 transport entry에서 적용한다. Proposal, Reservation과 Assignment는 target authority가 생성해야 하므로 이 첫 client-write surface에서 generic resource mutation으로 노출하지 않는다.

## Remaining Cutover Work

- identity provider와 credential/tenant lifecycle을 선택하고 authenticated remote listener를 구성한다.
- quota/rate limit, database pool/timeout과 numeric SLO를 실제 workload evidence로 정한다.

completed V0 import의 local backup/restore와 pre-writer rollback gate는 `scripts/check-postgres.sh`가 검증한다. 위 remaining 항목 전에는 `cmd/sema-server`를 PostgreSQL target runtime으로 바꾸거나 `v0alpha2`를 stable contract로 선언하지 않는다.
