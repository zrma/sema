# V0 Journal Import

## Scope

V0 `sema-journal-v1`은 optional compatibility import를 사용할 때 PostgreSQL repository로 복사되는 read-only source다. importer는 source를 in-place migration하거나 repair하지 않는다. 신규 설치는 V0 journal을 요구하지 않으며 이 fixture의 존재가 실제 predecessor deployment를 의미하지 않는다.

## Preconditions

1. V0 writer와 `cmd/sema-server`를 중지한다.
2. journal의 immutable backup과 checksum을 별도 operator evidence로 확보한다.
3. import 전용 빈 PostgreSQL schema/database와 하나의 target tenant scope를 준비한다.
4. positive batch size, server clock, import ID와 legacy tenant mapping을 명시한다.

V0 writer가 계속 append하는 source, incomplete tail, checksum/sequence/replay 오류, 빈 scope가 아닌 target은 import하지 않는다.

## Import Contract

`durable.ReadLegacyJournal`은 source를 read-only로 읽고 source byte digest를 계산한다. `service.LegacyImporter`는 journal을 다음 target resource로 normalize한다.

- immutable Policy
- active 또는 tombstoned MatchTicket/BackfillTicket과 demand/session claim
- reconstructed planning snapshot, completed planning run, proposal와 unmatched result
- active/terminal Reservation과 active claim 또는 released claim tombstone
- pending/terminal Assignment와 optional acknowledgment
- `legacy_import` start/completion marker

import start marker의 target storage version이 imported planning snapshot의 repository fence다. source path, credential, raw event와 private environment 식별자는 marker나 audit에 저장하지 않는다.

## Failure And Retry

completion marker 전 오류는 rollback 가능한 V0 source를 바꾸지 않는다. target에는 `importing` marker와 일부 batch가 남을 수 있으며 같은 scope에서 resume하거나 resource별 cleanup하지 않는다.

1. incomplete target schema/database를 폐기한다.
2. 새 빈 target을 migration한다.
3. 같은 immutable source와 import ID로 전체 import를 다시 실행한다.
4. `RequireLegacyImportCompleted`로 expected source digest, completed state와 marker storage version을 확인한다.

completed response만 유실된 경우 same source/import ID retry는 completed marker를 replay한다. 다른 source digest 또는 다른 preexisting resource가 있는 target은 재사용하지 않는다.

## Import Recovery

completion marker만으로 imported repository의 recovery readiness를 선언하지 않는다. `scripts/check-postgres.sh`의 local rehearsal은 imported target을 backup/삭제/restore하고 aggregate digest, completion marker와 terminal assignment를 확인한 뒤 target을 폐기해 V0를 digest 변화 없이 다시 연다. 이 단계는 optional import compatibility와 local logical recovery evidence이며 신규 설치나 실제 game traffic transition의 전제 조건이 아니다.

imported PostgreSQL repository에 새 mutation이 기록되기 전에는 V0 binary와 original journal backup으로 import 시도를 되돌릴 수 있다. 새 mutation이 기록된 뒤 reverse synchronization은 지원하지 않으므로 V0를 다시 writer로 열지 않는다. 그 시점의 recovery는 PostgreSQL backup restore와 compatible service binary의 책임이다.

리허설의 private manifest와 dump는 임시 operator artifact다. source path, DSN, raw payload와 environment identity는 tracked 문서나 일반 CI artifact에 남기지 않고 phase별 aggregate count만 보고한다.
