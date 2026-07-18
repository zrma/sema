# ADR 0022: Assignment Delivery And Acknowledgment Boundary

- Status: Accepted
- Date: 2026-07-18

## Context

reservation confirm은 단순 status 변경이 아니다. proposal이 참조한 demand freshness를 다시 검증하고 queue demand를 소비하며 pending assignment를 만들어야 한다. 이 동작이 여러 commit으로 나뉘면 ticket은 사라졌지만 assignment가 없거나, assignment는 둘인데 reservation은 하나인 상태가 생길 수 있다.

assignment를 받은 consumer는 game/session 적용을 소유한다. Sema가 external session을 직접 변경하거나 성공으로 추측하면 matchmaker authority와 game-runtime authority가 섞인다. 반대로 terminal acknowledgment 경쟁을 last-write-wins로 두면 서로 다른 결과가 동시에 관찰될 수 있다.

## Decision

- confirm은 active reservation, authoritative proposal, current ticket revision, backfill session/roster version과 reservation claim ownership을 다시 검증한다.
- successful confirm은 match/backfill demand tombstone, backfill session claim 해제, reservation claim 해제, confirmed reservation, pending assignment와 historical operation result를 한 repository transaction에 기록한다.
- freshness가 깨진 confirm은 assignment를 만들지 않고 internal invalidation operation으로 reservation을 cancel하며 claim 전체를 해제한 뒤 `StaleSnapshot`을 반환한다.
- assignment ID는 tenant scope에서 immutable하고 client는 confirm command에서 ID만 제안한다. placement는 authoritative proposal에서 복사한다.
- delivery baseline은 authenticated get/list polling이다. stream, broker와 outbox는 요구가 생기기 전까지 추가하지 않는다.
- consumer는 terminal outcome을 acknowledgment로 제출한다. HTTP `Idempotency-Key`가 acknowledgment operation ID authority이며 body가 별도 operation identity를 제출하지 않는다.
- acknowledgment는 pending assignment CAS update, immutable acknowledgment child와 historical operation result를 한 transaction에 기록한다.
- 서로 다른 terminal outcome 경쟁은 정확히 하나만 commit하고 loser는 `InvalidTransition`을 받는다.
- backfill completion 또는 stale failure는 assignment의 session ID와 expected roster version을 그대로 제시하고 resulting roster version을 전진시켜야 한다.
- acknowledgment는 external game/session 적용 결과의 기록이다. Sema는 외부 session을 mutation하지 않는다.

## Consequences

- confirm 뒤 queue demand는 active list에서 사라지고 backfill session은 새 demand가 다시 claim할 수 있다.
- confirm response가 유실되고 assignment가 이미 terminal이 되어도 original operation retry는 pending response를 재생한다.
- assignment read model은 current terminal outcome을 제공하지만 operation replay result와 역할이 다르다.
- polling snapshot cursor는 tenant repository version이 바뀌면 첫 page부터 다시 시작한다.
- external session side effect와 acknowledgment 사이 exactly-once는 consumer 책임이다. Sema는 idempotent result recording만 보장한다.

## Verification

- new-match confirm이 demand를 소비하고 reservation/assignment를 atomic하게 전환한다.
- backfill confirm이 ticket과 session claim을 해제하고 roster-version acknowledgment를 검증한다.
- separate repository handle의 concurrent terminal transition 중 하나만 성공하며 reopen 뒤 하나의 terminal state만 남는다.
- terminal acknowledgment 뒤 original confirm replay가 original pending response를 반환한다.
- authenticated memory/PostgreSQL HTTP fixture가 confirm, poll, acknowledgment와 historical replay를 같은 contract로 실행한다.

## Revisit Triggers

- consumer가 polling보다 강한 ordered delivery 또는 push retry를 요구한다.
- assignment side effect를 조정하는 transactional outbox/inbox가 필요해진다.
- assignment retention/privacy deletion이 operation replay horizon과 충돌한다.
