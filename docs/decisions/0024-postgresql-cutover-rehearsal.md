# ADR 0024: PostgreSQL Cutover Rehearsal

- Status: Accepted
- Date: 2026-07-18

## Context

V0 import completion marker는 source digest와 target normalization 완료를 증명하지만 PostgreSQL backup artifact가 복원 가능한지, 복원 뒤 operation/audit/resource 상태가 같은지, target writer 시작 전 V0로 실제 되돌아갈 수 있는지는 증명하지 않는다. 반대로 production provider, PITR product와 numeric recovery objective가 정해지지 않은 상태에서 특정 managed backup topology를 baseline으로 고정할 수도 없다.

리허설 evidence에는 raw resource payload, database credential, source path나 host/container identity를 남기지 않아야 한다. V0 rollback도 target write 시작 전과 후의 의미가 다르므로 하나의 양방향 migration처럼 표현하면 안 된다.

## Decision

- `cmd/sema-postgres-rehearsal`은 credential을 CLI나 manifest에 받지 않고 `SEMA_POSTGRES_TEST_DSN` 환경변수로만 읽는다.
- seed phase는 새 stopped V0 fixture를 만들고 read-only importer로 빈 tenant scope에 옮긴다. private temporary manifest에는 source digest/record count, repository version, resource/audit digest, metadata/scope/operation authority digest와 repository table별 row count만 mode `0600`으로 기록한다.
- reference backup은 pinned PostgreSQL container 안의 같은 major-version `pg_dump` custom format으로 isolated schema 전체를 캡처한다. 이것은 provider-neutral logical restore fixture이지 production backup product 선택이 아니다.
- 원본 target schema를 완전히 삭제한 뒤 `pg_restore`하고, migration을 다시 실행하지 않은 상태에서 manifest exact equality, import completion marker와 terminal assignment read를 확인한다.
- pre-writer rollback phase는 복원된 target schema를 폐기한 뒤 원본 V0 journal을 기존 reservation TTL로 다시 열어 readiness와 terminal assignment를 확인한다. open/close 전후 source digest와 record count가 달라지면 실패한다.
- `scripts/check-postgres.sh`는 repository/service/API integration과 위 seed, backup, destructive target restore, semantic verify, target discard, V0 restart 순서를 하나의 disposable local gate에서 실행한다.
- stdout은 phase와 aggregate count만 담은 redacted report다. journal, manifest, dump, DSN과 실제 operator inventory는 CI artifact나 tracked 문서에 보존하지 않는다.

## Consequences

- completed import를 실제로 backup/restore한 뒤 source-preserving pre-writer rollback이 가능하다는 executable evidence가 생긴다.
- schema 일부나 resource table만 복사해 operation receipt/audit가 빠지는 backup은 manifest comparison에서 거부된다.
- 이 fixture는 test container의 logical dump/restore를 증명할 뿐, production provider의 encryption, retention, cross-region copy, PITR, RPO/RTO 또는 restore throughput을 보증하지 않는다.
- target writer가 첫 mutation을 승인받은 뒤 V0를 writer로 재개할 수 없다. 이후 rollback은 target PostgreSQL backup과 compatible target binary를 사용해야 한다.
- remote target writer cutover는 여전히 identity provider, tenant credential lifecycle, TLS termination owner와 abuse boundary가 결정된 뒤에만 가능하다.

## Alternatives Rejected

- dump command 성공만 확인: restored semantic state와 source rollback을 증명하지 못한다.
- repository resource table만 export: operation idempotency receipt, scope version과 audit authority를 잃는다.
- import 직후 V0 writer와 target writer를 동시에 시작: reverse synchronization 계약이 없고 split authority를 만든다.
- 특정 managed PostgreSQL backup product를 지금 채택: deployment/provider evidence가 없는 선택을 core repository contract에 결합한다.
