# ADR 0031: Standard Service Recovery Acceptance

## Status

Accepted

## Context

optional V0 import는 logical backup/restore와 source rollback을 검증하지만, V0를 사용하지 않는 신규 PostgreSQL service의 checkpoint consistency, operation receipt와 복원 후 write readiness를 증명하지 않는다. 반대로 특정 managed database나 WAL product를 repository baseline으로 선택할 deployment evidence도 없다.

## Decision

- native standard lifecycle 전용 `sema-postgres-recovery` seed/advance/verify command를 추가한다.
- seed는 terminal assignment까지 기록하고 snapshot/resource/audit 및 metadata/scope/operation authority를 private manifest로 캡처한다.
- repository gate는 checkpoint 뒤 mutation을 만든 다음 원본 schema를 삭제하고 pinned same-major logical dump를 복원한다.
- verify는 current migration compatibility, checkpoint exact equality, post-checkpoint exclusion, idempotent operation replay와 terminal assignment를 확인한다.
- 복원 repository 위에 stateless target runtime을 시작해 readiness/read와 새 API write가 authority version을 전진시키는지 확인한다.
- sanitized report만 CI artifact로 보존하고 manifest/dump/DSN/schema/resource identity는 폐기한다.
- deployment PITR 제품은 seed와 advance 사이로 복원한 뒤 같은 verify를 사용하고 자체 RPO/RTO를 기록한다.

## Consequences

신규 설치는 V0 import와 무관하게 destructive restore를 거친 semantic recovery evidence를 갖는다. repository는 logical checkpoint reference와 PITR acceptance protocol을 제공하지만 backup scheduler, WAL retention, encryption, geographic copy, provider failover와 numeric RPO/RTO는 소유하지 않는다.

## Rejected Alternatives

- 기존 V0 import rehearsal을 standard recovery로 간주: native writer와 복원 후 service write를 검증하지 않는다.
- `pg_dump`/`pg_restore` exit code만 확인: operation receipt와 lifecycle semantics 유실을 찾지 못한다.
- 특정 managed PostgreSQL PITR 제품 채택: core contract가 provider와 private deployment inventory에 종속된다.
- restore target을 즉시 writer로 전환: 격리 semantic verification 전에 authority가 바뀐다.

## Revisit When

- repository schema migration이 multiple deployed versions를 함께 지원해야 한다.
- measured recovery volume이 logical reference gate의 실행 시간을 비현실적으로 만든다.
- deployment evidence가 repository-owned numeric recovery profile을 정당화한다.
