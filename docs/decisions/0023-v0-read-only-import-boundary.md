# ADR 0023: V0 Read-Only Import And Discard Boundary

- Status: Accepted
- Date: 2026-07-18

## Context

V0 `sema-journal-v1` open 경로는 single-writer recovery를 위해 file lock, permission normalization과 incomplete tail truncate를 수행한다. 이 경로를 importer가 사용하면 source를 수정하지 않는 rollback 계약을 위반한다. 또한 긴 journal을 한 PostgreSQL transaction으로 옮기면 transaction/resource budget을 예측하기 어렵지만 batch 중단 뒤 부분 target을 정상 writer로 오인해서도 안 된다.

V0 plan은 repository version이 없고 journal event sequence만 가진다. target planning snapshot은 repository version fence를 요구한다. migration이 이 차이를 숨기면 imported run의 source version 의미가 잘못 해석된다.

## Decision

- importer는 V0 runtime open/recovery 경로를 사용하지 않고 regular file을 read-only로 한 번 읽는다.
- source record sequence, checksum, schema와 full replay를 검증한다. incomplete tail은 truncate/repair하지 않고 import를 거부한다.
- read 전후 size/mtime과 read byte count가 다르면 concurrent writer로 판단해 거부한다. cutover rehearsal은 V0 writer를 먼저 중지해야 한다.
- source 전체 byte digest를 import identity와 completion marker에 저장한다. source path와 raw payload는 target marker/audit에 저장하지 않는다.
- import는 repository scope가 완전히 비어 있을 때만 시작한다.
- 첫 commit은 `legacy_import` marker를 `importing`으로 만들고 그 storage version을 imported planning snapshot의 target import fence로 사용한다. V0 event sequence를 target repository version으로 가장하지 않는다.
- 검증된 source를 normalized target resource로 만든 뒤 caller가 명시한 bounded batch 크기로 commit한다.
- 마지막 commit만 marker를 `completed`로 전환하고 source digest, record count와 imported resource count를 확정한다.
- response loss 뒤 같은 source/import ID를 다시 실행하면 completed marker를 replay한다.
- batch 중단 뒤 `importing` marker가 남은 scope는 resume하지 않는다. `ErrLegacyImportIncomplete`로 거부하고 isolated target scope/schema 전체를 폐기한 뒤 빈 target에 다시 실행한다.
- target writer cutover는 `RequireLegacyImportCompleted`가 expected source digest와 completed marker를 확인한 뒤에만 허용한다.
- importer는 durable plan event가 없는 directly forged reserved proposal을 target authority로 승격하지 않고 거부한다.

## Consequences

- source journal은 bytes, mode, size와 mtime이 import 전후 동일하게 유지된다.
- partial batch 사이에 관계가 잠시 불완전할 수 있지만 completed marker 전 target은 serving/cutover 대상이 아니다.
- discard 단위는 tenant scope보다 강한 isolated schema/database가 권장된다. 같은 scope resource를 부분 삭제해 재사용하지 않는다.
- imported planning run의 `source_repository_version`은 V0 event sequence가 아니라 import start marker의 target repository version이다. exact V0 input은 imported planning snapshot payload가 보존한다.
- V0 operation receipt는 target client idempotency receipt로 가장하지 않는다. imported aggregate/current terminal state와 journal digest가 migration evidence다.
- source가 import 뒤 다시 write되면 digest가 달라지므로 해당 target은 그 후속 event를 포함하지 않는다. writer stop/cutover ordering은 operator gate다.

## Verification

- valid source load/import 전후 source bytes와 metadata가 동일하다.
- incomplete tail은 source를 repair하지 않고 거부한다.
- memory target이 policy, demand tombstone/active state, planning result, confirmed reservation과 terminal assignment를 복원한다.
- injected batch failure는 `importing` marker를 남기고 same target retry를 거부하며 fresh target 재실행은 완료된다.
- PostgreSQL batch import completion과 terminal assignment가 store reopen 뒤에도 유지된다.

## Revisit Triggers

- representative journal이 current in-memory normalization budget을 넘는다.
- resume가 schema discard보다 운영상 유리하다는 production evidence가 생긴다.
- V0 source가 하나의 tenant가 아닌 multi-tenant mapping을 요구한다.
