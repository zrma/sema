# Service Authority And Failure Contract

## Resource Authority

| Resource | Durable authority | Minimum lifetime | Notes |
|---|---|---|---|
| Policy | repository | 이를 참조하는 snapshot/proposal이 존재하는 동안 | version은 tenant scope 안에서 immutable fingerprint에 묶인다. |
| Match ticket | repository | active 또는 reserved 상태와 retry/import horizon 동안 tombstone 포함 | domain revision과 repository storage version은 별개다. |
| Backfill ticket | repository | active 또는 reserved 상태와 retry/import horizon 동안 tombstone 포함 | ticket revision과 roster version을 함께 검증한다. |
| Planning snapshot | repository | planning run, proposal, reservation과 audit가 참조하는 동안 | matcher에 전달되는 immutable input과 repository version을 보존한다. |
| Planning run | repository | proposal/unmatched audit 및 idempotency horizon 동안 | matcher 계산은 storage transaction 밖에서 실행한다. |
| Proposal | repository | reservation/assignment 및 plan audit가 참조하는 동안 | client가 제출한 placement는 authority가 아니다. |
| Reservation | repository | active/expiry 뒤 assignment 및 retry horizon 동안 | server clock expiry와 ticket/backfill claim을 한 transaction에서 변경한다. |
| Assignment/acknowledgment | repository | consumer retry, support와 audit horizon 동안 | external session 적용 결과만 acknowledgment로 기록한다. |
| Operation/audit receipt | repository commit log | maximum retry 및 operator audit horizon보다 짧지 않게 | idempotency identity는 private receipt에 두고 기본 audit에는 bounded command/resource kind count와 time만 남긴다. |
| Demand identity claim | repository | ticket tombstone retry/import horizon보다 짧지 않게 | MatchTicket/BackfillTicket이 tenant 안에서 같은 ID를 동시에 사용하지 못하게 한다. |
| Backfill session claim | repository | active backfill과 cancellation commit까지 | session별 active backfill 하나를 보장하고 exact cancel에서 해제한다. |
| Demand reservation claim | repository | active reservation terminal transition까지 | proposal demand별 단일 active reservation을 PostgreSQL CAS로 보장한다. |
| Operation result | repository | operation receipt retry horizon보다 짧지 않게 | 후속 aggregate mutation 뒤에도 최초 mutation 응답을 재생한다. |
| Candidate index | derived repository owner | retention 없음 | exact repository version에서만 사용하고 언제든 snapshot으로 rebuild한다. |

정확한 duration은 실제 consumer retry, privacy deletion과 support SLO가 없으므로 아직 숫자로 고정하지 않는다. adapter는 위 reference가 살아 있는 동안 dependency를 먼저 삭제할 수 없다.

## Transaction Sequence

1. authenticated principal과 tenant scope를 먼저 결정한다.
2. canonical command payload에서 idempotency digest를 만든다.
3. repository의 consistent snapshot에서 필요한 resource와 storage version을 읽는다.
4. planning 요청이면 immutable planning snapshot을 저장한 뒤 matcher를 transaction 밖에서 실행한다.
5. planning completion은 deterministic result digest로 run, proposal와 unmatched resource 전체를 한 commit에 기록한다.
6. mutation command는 읽은 resource version을 CAS precondition으로 제출한다.
7. resource changes, operation receipt와 redacted audit receipt를 한 commit으로 기록한다.
8. 응답이 유실되면 같은 operation ID와 digest를 재시도해 최초 commit receipt를 읽는다.

historical response가 current aggregate에서 재구성되지 않는 reservation/assignment command는 immutable internal `operation_result`도 같은 commit에 기록한다. replay는 receipt version으로 operation을 확인한 뒤 이 result를 읽는다.

reserve와 confirm은 planning 당시 queue version 전체를 잠그지 않는다. proposal이 참조한 exact ticket revision, backfill roster version, 현재 claim과 reservation status만 atomic하게 다시 검증한다. 관련 없는 queue ingress는 reserve를 무효화하지 않는다.

## Failure Matrix

| Scenario | Atomic outcome | Service failure/retry | Executable evidence |
|---|---|---|---|
| 같은 ticket storage version을 두 update가 경쟁 | 정확히 하나만 commit | loser는 command 의미에 맞게 `InvalidRevision` 또는 `StaleSnapshot` | `repositorytest` same ticket revision competition |
| 한 reserve의 여러 ticket 중 하나가 stale | 어떤 ticket/reservation도 부분 변경하지 않음 | `StaleSnapshot` 또는 `ReservationConflict`; 새 snapshot에서 replan | `repositorytest` atomic multi resource conflict, `TestReserveRejectsStaleTicketRevisionAtomically` |
| 같은 operation ID와 같은 digest | 최초 commit version/result 반환 | 안전한 retry | `repositorytest` duplicate operation receipt |
| 같은 operation ID와 다른 digest | mutation 없음 | `IdempotencyConflict`; 새 ID 또는 caller bug 수정 | `repositorytest` duplicate operation receipt |
| reservation expiry 이전/이후 confirm 경쟁 | server clock 기준 한 terminal transition만 commit | expiry가 이기면 `ReservationExpired`; claim 전체 해제 | `TestConfirmRejectsExpiredReservation`, `TestReservationExpiryThroughEngineReleasesWholeProposal` |
| commit 전 process crash | operation receipt와 mutation 모두 없음 | 같은 command retry가 새 commit 수행 | file prototype subprocess `after_temp_sync` fixture |
| commit 후 response 전 process crash | receipt와 mutation 모두 존재 | 같은 ID/digest retry가 최초 result 반환 | file prototype subprocess `after_commit`, repository reopen conformance |
| plan 뒤 ticket revision/roster 변경 | immutable plan/audit은 남고 reserve는 적용되지 않음 | `StaleSnapshot`; 새 snapshot에서 replan | `TestConfirmRepeatsCASAndReleasesStaleReservation`, `TestReserveRejectsStaleBackfillRoster` |
| candidate index가 repository보다 뒤처짐 | stale index 결과를 사용하지 않음 | delta reconcile 또는 snapshot rebuild 뒤 retry | `TestCandidateIndexRejectsAnotherRepositoryVersion` |
| 서로 다른 terminal acknowledgment 경쟁 | 하나의 terminal state만 commit | 동일 operation retry는 기존 result, 다른 transition은 `InvalidTransition` | `TestConcurrentAssignmentTerminalTransitionHasOneWinner` |
| cross-tenant resource 접근 | repository key/query 실행 전 거부 | 인증/인가 오류; resource existence를 누출하지 않음 | `TestTargetAPIAuthenticatesAuthorizesAndIsolatesTenants` |
| Match/Backfill 동일 ID 경쟁 | demand payload와 identity claim 중 한 transaction만 commit | loser는 `InvalidRevision`; partial demand 없음 | `TestDemandIdentityClaimSerializesMatchAndBackfillCompetition` |
| 동일 session Backfill 경쟁 | session claim CAS 중 한 transaction만 commit | loser는 `InvalidInput`; cancel 뒤 새 ID로 retry | `TestBackfillSessionClaimSerializesCompetitionAndReleasesOnCancel` |
| snapshot capture 뒤 matcher 중단 | `planning` run과 immutable snapshot만 남고 partial result는 없음 | 같은 client operation retry가 저장된 snapshot으로 resume | `TestPlanningRunResumesCapturedSnapshotAfterPlannerInterruption` |
| matcher 실행 중 unrelated queue ingress | ingress commit은 matcher를 기다리지 않고 captured input에는 섞이지 않음 | 다음 planning run에서 새 demand를 포함 | `TestPlanningRunReleasesRepositoryWhileMatcherComputes` |
| 같은 proposal reserve 경쟁 | reservation과 demand claim 전체 중 한 transaction만 commit | loser는 `ReservationConflict`; cancel/expiry 뒤 retry | `TestCompetingReservationsClaimProposalDemandAtomically` |
| reservation 뒤 demand revision 변경 | reservation과 claim이 생성되지 않음 | `StaleSnapshot`; 새 planning run에서 retry | `TestReservationRejectsDemandChangedAfterPlanning` |
| reserve 뒤 cancel 후 original retry | current terminal resource와 별개로 original active response 유지 | same operation ID/digest로 original storage version 반환 | `TestReservationAPIClaimsCancelsListsAndReplays` |
| confirm과 demand mutation 경쟁 | demand 소비, reservation confirm과 assignment create 전체가 commit되거나 전혀 적용되지 않음 | loser는 `ReservationConflict` 또는 `StaleSnapshot`; 새 plan에서 retry | `TestStaleConfirmCancelsReservationAndReleasesClaims` |
| 서로 다른 terminal acknowledgment 경쟁 | pending assignment에서 하나의 outcome만 commit | loser는 `InvalidTransition`; current assignment poll | `TestConcurrentAssignmentTerminalTransitionHasOneWinnerInRepositoryService` |
| confirm 뒤 terminal ack 후 original confirm retry | current terminal read model과 별개로 original pending response 유지 | same operation ID/digest로 original storage version 반환 | `TestAssignmentAPIConfirmsPollsAcknowledgesAndReplays` |

## API Resource Model

모든 target identity와 operation ID의 idempotency 범위는 tenant scope 안이다. resource key는 `(tenant scope, resource kind, resource ID)`다. 첫 experimental transport 이름은 `v0alpha2`이며 stable compatibility는 consumer review 전까지 확정하지 않는다. 다음 operation semantics는 adapter와 무관하게 유지하고 persistent prototype과 storage 결정 evidence는 `docs/repository-adapter-evidence.md`가 소유한다.

| Resource | Commands | Reads and delivery |
|---|---|---|
| Policy | immutable create | get/list; fingerprint 포함 |
| Match ticket | create, higher-revision replace, exact cancel | get/list active; cursor pagination |
| Backfill ticket | create, higher-revision/roster replace, exact cancel | get/list active; cursor pagination |
| Planning run | idempotent create | get status; immutable snapshot reference, paged proposal/unmatched 결과 |
| Proposal | planning run만 생성 | get/list by planning run; client placement write 금지 |
| Reservation | idempotent create from proposal, confirm, cancel | get/list active or terminal; expiry는 server clock |
| Assignment | reservation confirm만 생성 | get/list/poll pending or terminal |
| Acknowledgment | assignment child로 idempotent create | assignment read model에 terminal outcome 반영 |
| Audit receipt | repository commit만 생성 | tenant-scoped opaque cursor로 redacted page; operation/resource ID와 raw payload 기본 제외 |

모든 mutation은 operation ID를 요구한다. list cursor는 tenant, filter와 stable repository order에 묶여야 하며 client가 다른 tenant/filter에 재사용할 수 없다. 최초 target delivery는 polling과 cursor pagination을 보장하고 stream/outbox는 consumer requirement가 생길 때 별도 capability로 추가한다.

remote listener는 authenticated principal과 tenant authorization을 요구한다. provider/protocol은 미정이지만 unauthenticated remote opt-out은 target surface에 포함하지 않는다. quota와 rate-limit 수치는 workload evidence 뒤 정하되 enforcement point는 repository transaction 전이다.

## PostgreSQL Authority

ADR 0017에 따라 PostgreSQL primary가 target mutation authority다. write는 resource row를 canonical order로 잠가 CAS를 확인하고 scope version을 commit 직전에 배정한 뒤 operation receipt, resource mutation과 audit receipt를 한 transaction으로 확정한다. read snapshot은 read-only Repeatable Read에서 version과 resource set을 함께 읽는다. service replica, candidate index와 Redis는 durable authority가 아니며 Redis는 baseline에서 사용하지 않는다. schema와 검증 계약은 `docs/postgres-repository.md`가 소유한다.

## V0 HTTP Migration Mapping

| V0 `v0alpha1` endpoint | Target resource operation | Migration note |
|---|---|---|
| `PUT /policies/{version}` | Policy create | configured legacy tenant로 scope하고 fingerprint conflict를 보존한다. |
| `PUT /match-tickets/{id}` | Match ticket create/replace | explicit operation ID를 추가하고 domain revision과 storage version을 분리한다. |
| `DELETE /match-tickets/{id}` | Match ticket exact cancel | tombstone과 operation receipt를 보존한다. |
| `PUT /backfill-tickets/{id}` | Backfill ticket create/replace | roster version CAS와 optional roster aggregate를 그대로 import한다. |
| `DELETE /backfill-tickets/{id}` | Backfill ticket exact cancel | ticket/roster freshness를 함께 검증한다. |
| `POST /plans` | Planning run create | V0 snapshot ID는 legacy operation/resource ID로 import하고 immutable planning snapshot을 분리한다. |
| `POST /reservations/{id}` | Reservation create from proposal | proposal body가 아닌 authoritative proposal reference만 받는다. |
| `POST /reservations/{id}/confirm` | Reservation confirm | assignment create와 ticket consumption을 한 transaction에 둔다. |
| `POST /reservations/{id}/cancel` | Reservation cancel | active claim 전체를 한 transaction에서 해제한다. |
| `GET /assignments/{id}` | Assignment get/poll | same assignment identity와 terminal read model을 보존한다. |
| `POST /assignments/{id}/acknowledgments` | Acknowledgment create | V0 operation ID와 roster CAS outcome을 보존한다. |
| `GET /audit` | Audit receipt page | numeric `after`는 target opaque cursor로 변환하고 raw journal payload는 노출하지 않는다. |

V0 importer는 source journal을 수정하지 않는다. import target이 완성되기 전에는 V0 writer를 다시 시작할 수 있고, cutover 뒤 reverse write는 지원하지 않는다. target cutover와 V0 폐기는 별도 migration acceptance 및 backup/restore rehearsal 뒤에만 수행한다.
