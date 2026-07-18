# ADR 0021: Reservation Claim And Historical Replay Authority

- Status: Accepted
- Date: 2026-07-18

## Context

planning result는 immutable proposal이지만 그 proposal이 참조한 MatchTicket과 BackfillTicket은 planning transaction 뒤에도 변경되거나 다른 proposal이 먼저 선점할 수 있다. replica-local lock이나 reservation row만 만들면 서로 다른 reservation row가 같은 demand를 동시에 소유하는 경쟁을 막지 못한다.

또한 operation receipt가 commit version과 digest만 보존하면 reserve 뒤 cancel/expiry처럼 resource가 바뀐 후 최초 reserve retry에 당시 응답을 재구성할 수 없다. current reservation을 읽어 반환하면 historical idempotency가 깨진다.

## Decision

- reserve command는 client placement가 아니라 repository가 보존한 `proposal` ID만 받는다.
- proposal의 모든 match ticket과 optional backfill target freshness를 현재 resource에서 다시 검증한다.
- `demand_reservation_claim` resource를 ticket ID별로 만들고 reservation, 모든 claim과 operation result를 한 transaction에서 commit한다.
- 서로 다른 reservation이 같은 demand를 경쟁하면 claim CAS로 하나만 commit한다. Redis와 process-local lease는 correctness authority가 아니다.
- cancel과 server-clock expiry는 reservation terminal status와 모든 claim tombstone을 한 transaction에서 commit한다.
- tombstoned claim은 storage version CAS로 다시 사용할 수 있다. demand identity 자체는 재사용하지 않는다.
- client mutation의 exact historical 응답은 immutable `operation_result` resource에 저장한다. receipt replay는 current aggregate가 아니라 해당 result를 읽는다.
- reservation TTL은 target handler composition에서 양수로 명시해야 하며 library default로 임의 확정하지 않는다.
- expiry는 읽기 또는 command 진입에서 server clock으로 발견하고 internal deterministic operation으로 영속화한다. 만료된 claim이 새 reserve를 막지 않는다.

## Consequences

- 한 reserve transaction은 reservation, proposal에 포함된 demand 수만큼의 claim과 operation result를 함께 변경한다.
- reservation list/get은 만료를 발견하면 read-only 동작에 앞서 expiry commit을 수행할 수 있다.
- terminal reservation은 audit/retry reference를 위해 유지하고 같은 reservation ID를 새 command가 재사용할 수 없다.
- `operation_result`와 reservation claim은 repository-internal resource이며 generic client write/list surface에 노출하지 않는다.
- exact TTL, cleanup batch, retention과 background expiry sweep은 deployment/retry evidence가 생기면 조정한다.

## Verification

- separate memory repository handle 두 개가 같은 proposal을 reserve하면 하나만 성공한다.
- cancel과 expiry 뒤 다른 reservation이 같은 proposal demand를 claim한다.
- plan 뒤 ticket revision이 바뀌면 reservation과 claim이 부분 생성되지 않는다.
- cancel 뒤 최초 reserve operation을 replay해도 original active status와 storage version을 반환한다.
- authenticated API가 tenant/permission, get/list, cancel, expiry와 historical replay를 검증한다.

## Revisit Triggers

- representative large match의 claim fan-out이 PostgreSQL transaction budget을 넘는다.
- consumer가 eager background expiry 또는 lease renewal을 요구한다.
- privacy/retention 요구가 terminal reservation과 immutable operation result lifetime을 제한한다.
