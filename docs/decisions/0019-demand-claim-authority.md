# ADR 0019: Demand Claim Authority

- Status: Accepted
- Date: 2026-07-18

## Context

`MatchTicket`과 `BackfillTicket`은 repository에서 resource kind가 다르지만 domain에서는 하나의 ticket identity namespace를 공유한다. 또한 같은 session에 active backfill demand가 둘 존재하면 서로 다른 vacancy/roster snapshot이 동시에 proposal authority가 될 수 있다.

service replica가 snapshot에서 중복을 확인하기만 하면 서로 다른 resource row를 동시에 create하는 경쟁을 막을 수 없다. process-local lock이나 Redis claim을 추가하면 PostgreSQL transaction과 별도 correctness authority가 생긴다.

## Decision

- tenant마다 `demand_identity` resource가 MatchTicket/BackfillTicket의 공통 ticket identity를 claim한다.
- demand identity claim은 최초 demand create와 같은 transaction에서 생성하고 cancellation 뒤에도 보존한다. tombstone ticket identity를 다른 demand kind나 새 demand로 재사용하지 않는다.
- tenant마다 `backfill_session_claim` resource가 session의 유일한 active BackfillTicket을 가리킨다.
- backfill session claim은 BackfillTicket create와 같은 transaction에서 생성하고 exact cancellation과 같은 transaction에서 tombstone으로 해제한다.
- 해제된 session은 새 ticket identity가 tombstoned session claim의 storage version을 CAS해 다시 claim할 수 있다.
- replacement는 demand identity와 session claim ownership을 검증하되 claim version을 불필요하게 갱신하지 않는다.
- claim conflict는 PostgreSQL resource CAS로 결정한다. Redis, application lease와 replica-local lock은 사용하지 않는다.
- BackfillTicket replacement는 ticket revision과 roster version이 모두 단조 증가해야 한다. 같은 roster version에서 vacancy/roster context를 바꿀 수 없고 session identity는 변경할 수 없다.

## Consequences

- create transaction은 demand payload 외에 최대 두 claim resource를 함께 변경한다.
- 다른 kind의 동일 ticket ID 경쟁과 같은 session의 서로 다른 backfill 경쟁은 여러 replica에서도 하나만 commit한다.
- BackfillTicket cancellation은 ticket tombstone과 session claim tombstone을 atomic하게 기록한다.
- claim은 client API resource가 아니며 generic mutation/list surface에 노출하지 않는다.
- identity claim retention은 ticket tombstone retry/import horizon보다 짧을 수 없다.

## Verification

- shared memory backend의 separate service handle 경쟁에서 MatchTicket/BackfillTicket 중 하나만 같은 identity를 얻는다.
- 서로 다른 BackfillTicket이 같은 session을 경쟁하면 하나만 성공하고 partial demand가 남지 않는다.
- winner cancellation 뒤 새로운 ticket이 같은 session을 claim한다.
- PostgreSQL target API fixture가 claim을 포함한 MatchTicket 및 BackfillTicket create/poll을 실제 transaction으로 실행한다.

## Revisit Triggers

- product가 ticket identity reuse 또는 explicit resurrection을 요구한다.
- 하나의 session이 partitioned vacancy authority를 여러 BackfillTicket으로 분리해야 한다.
- retention/privacy deletion이 permanent identity claim과 충돌한다.
