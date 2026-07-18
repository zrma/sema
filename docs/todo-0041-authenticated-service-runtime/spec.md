# P30 Authenticated Service Runtime Spec

- Status: In Progress — Reservation Boundary Complete

## Objective

P29의 PostgreSQL authority와 authenticated match-ticket vertical slice를 proposal/reservation/assignment lifecycle까지 확장하고, V0 journal을 rollback 가능한 import source로 유지하면서 target runtime cutover acceptance를 만든다.

## Fixed Inputs

- PostgreSQL primary만 durable mutation authority다. Redis, broker와 process-local lease는 correctness authority가 아니다.
- tenant는 authenticated principal에서만 결정하고 모든 mutation은 operation ID/digest를 가진다.
- matcher는 immutable repository-versioned snapshot 밖에서 계산하고 reserve/confirm은 current revision/roster CAS를 다시 검증한다.
- Proposal, Reservation과 Assignment는 client가 generic resource write로 생성하지 않는다.
- `v0alpha1` journal/API는 target cutover 전까지 별도 V0 reference/import source다.

## Implementation Sequence

- [x] BackfillTicket create/replace/exact-cancel/get/list를 match-ticket과 같은 tenant/idempotency contract로 연결한다.
- [x] immutable Policy create/get/list와 fingerprint conflict를 tenant-scoped repository authority로 연결한다.
- [x] planning run이 immutable snapshot을 저장하고 transaction 밖에서 matcher를 실행한 뒤 proposal/unmatched page를 기록하게 한다.
- [x] proposal-derived reservation create/cancel/get/list와 demand claim/expiry/historical replay를 repository multi-resource CAS로 구현한다.
- [ ] reservation confirm과 assignment polling/acknowledgment를 repository multi-resource CAS로 구현한다.
- [ ] V0 journal read-only import와 completion marker, discard-and-retry failure fixture를 만든다.
- [ ] 선택된 identity provider adapter, credential lifecycle, TLS와 remote-listener gate를 구성한다.
- [ ] backup/restore 및 V0 rollback rehearsal 뒤에만 target writer cutover를 승인한다.

## Acceptance

- memory와 PostgreSQL adapter에서 같은 full lifecycle service fixture가 통과한다.
- two-replica contention이 ticket/backfill double claim, forged proposal와 terminal assignment split-brain을 만들지 않는다.
- API page/poll, typed failure와 historical idempotency가 process restart 및 후속 mutation 뒤에도 유지된다.
- import 중 crash는 V0 source를 바꾸지 않고 incomplete target을 명확히 폐기하거나 재개한다.
- authenticated remote listener는 tenant/permission, TLS, secret loading과 bounded abuse control 없이 시작되지 않는다.
- cutover/rollback과 backup/restore operator evidence가 redacted aggregate로 남는다.

## Out Of Scope

- stable v1 wire/SDK compatibility 선언.
- consumer evidence 없는 streaming, outbox, Redis와 cross-region multi-primary.
- production provider/traffic 없이 임의의 numeric quota, SLO 또는 retention 확정.

## Decision Gate

provider-neutral planning run부터 assignment command service와 V0 import fixture는 계속 구현할 수 있다. 실제 remote executable과 credential 배포를 시작하기 전에는 identity provider, tenant credential lifecycle과 TLS termination owner를 사용자가 선택해야 한다. production PostgreSQL provider/backup topology와 numeric SLO는 별도 deployment evidence에서 결정한다.
