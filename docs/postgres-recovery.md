# PostgreSQL Recovery Acceptance

## Boundary

표준 service recovery는 신규 PostgreSQL 설치의 native `v0alpha2` lifecycle을 대상으로 한다. optional V0 import의 source-preserving rehearsal은 `docs/v0-import.md`가 별도로 소유한다.

repository gate는 provider-neutral logical checkpoint를 실제로 복원한다. managed database의 WAL archive, retention, encryption, cross-region copy, failover, RPO/RTO 또는 backup schedule을 구현하거나 보증하지 않는다.

## Executable Sequence

`scripts/check-postgres.sh`는 disposable database/schema에서 다음 순서를 실행한다.

1. current migration을 설치하고 2x2 match를 plan, reserve, confirm, terminal acknowledge한다.
2. repository snapshot/resource/audit와 metadata/scope/operation receipt authority의 private manifest를 mode `0600`으로 기록한다.
3. isolated schema 전체를 pinned PostgreSQL major의 custom-format `pg_dump`로 checkpoint한다.
4. checkpoint 이후 ticket mutation을 추가한다.
5. 원본 schema를 완전히 삭제하고 checkpoint dump를 복원한다.
6. current migration을 idempotently 재실행하고 checkpoint manifest exact equality를 확인한다.
7. checkpoint 이후 mutation이 없고 terminal assignment와 기존 operation receipt replay가 보존됐는지 확인한다.
8. 복원된 repository 위에 stateless target runtime을 시작해 readiness/assignment read와 새 API write를 실행한다.

성공 artifact `postgres-recovery.json`은 `sema.postgres-recovery.v1`이며 checkpoint aggregate와 각 판정 boolean만 포함한다. DSN, schema, resource ID, payload, digest, token, host/container identity와 private manifest는 artifact에 넣지 않는다.

## Deployment PITR Drill

실제 deployment의 backup/PITR 제품은 같은 semantic sequence를 소유해야 한다. `sema-postgres-recovery`의 seed/advance/verify 사이에서 logical dump 대신 deployment backup과 WAL recovery를 사용한다.

1. acceptance 전용 database/schema에서 seed를 완료하고 checkpoint manifest를 private evidence에 보관한다.
2. backup/PITR system이 seed transaction을 포함하는 복구 지점을 만들었음을 확인한다.
3. advance를 실행한 뒤 seed와 advance 사이의 target time으로 격리 database를 복원한다.
4. service writer를 연결하기 전에 verify를 실행한다.
5. measured recovery-point lag와 restore duration을 deployment evidence에 기록한다.
6. verify가 통과한 isolated target만 rollout rehearsal에 사용하고 원본/복원 target을 동시에 writer로 열지 않는다.

`SEMA_POSTGRES_TEST_DSN`, safe isolated schema와 private manifest가 필수이므로 command를 shared 또는 production writer database에 직접 실행하지 않는다. 실제 RPO/RTO threshold와 retention은 capacity, 비용, provider 계약과 incident ownership을 가진 deployment가 정한다.

## Failure Meaning

| Failure | Meaning |
|---|---|
| manifest mismatch | resource, audit, metadata, scope version, operation receipt 또는 table completeness가 checkpoint와 다르다 |
| post-checkpoint mutation exists | 요청한 recovery target보다 뒤의 write가 포함됐다 |
| operation replay fails | idempotency authority가 복구되지 않았다 |
| terminal assignment incomplete | lifecycle read model이 semantic checkpoint를 보존하지 못했다 |
| readiness/API write fails | 복원 DB가 current stateless service의 usable write authority가 아니다 |

dump/restore command의 exit code만 성공하거나 table 일부만 복사한 상태는 acceptance가 아니다. 실패한 target을 수동 보정해 writer로 승격하지 않고 backup 범위와 recovery target을 수정해 격리 복원부터 다시 실행한다.
