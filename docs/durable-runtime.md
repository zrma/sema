# Durable Runtime

## Scope

`internal/durable.Runtime`은 기존 in-memory engine을 single-writer append-only journal로 감싼다. policy, ticket ingestion, planning decision, reservation, assignment와 acknowledgment를 같은 순서로 기록하고 재시작 때 replay한다.

이 package는 P9 service adapter의 state authority이며 public `alpha` API나 wire DTO 자체가 아니다.

## Durability Contract

- mutation과 planning result는 checksummed `sema-journal-v1` record가 append되고 `fsync`된 뒤에만 caller에게 성공을 반환한다.
- sync 전 process가 종료되면 caller는 성공을 받지 못하며 같은 idempotency identity로 재시도한다.
- sync 뒤 response 전에 process가 종료되면 replay가 state를 복구하고 같은 reservation, assignment와 acknowledgment retry가 기존 결과를 반환한다.
- append 실패가 process 안에서 관측되면 journal을 다시 읽어 in-memory engine을 복구한 뒤 오류를 반환한다. 복구도 실패하면 runtime은 operator recovery가 필요한 poisoned state가 된다.
- 마지막 newline이 없는 tail만 torn write로 간주해 제거한다. checksum, sequence, schema가 잘못된 complete record는 자동으로 건너뛰지 않고 startup을 실패시킨다.

첫 record는 reservation TTL을 nanosecond 단위로 고정한다. 재시작 configuration이 다르면 expiry semantics가 바뀌므로 startup을 거부한다.

## Audit Record

각 record는 schema, monotonic sequence, kind, payload와 SHA-256 checksum을 가진다. `Audit(after, limit)`은 최대 1000개를 defensive copy로 반환한다.

`plan_completed`는 proposal/evidence와 unmatched 목록을 포함한 batch 전문, canonical unmatched digest와 budget outcome을 기록한다. `snapshot_id`는 durable idempotency key이므로 response가 유실된 뒤 재시작해도 최초 batch를 그대로 반환한다. 이 완전성은 large queue journal growth를 키우므로 pagination/retention은 P10에서 측정해 보완한다.

## Storage And Ownership

- journal file mode는 `0600`, 새 parent directory는 `0700`이다.
- Darwin/Linux advisory file lock으로 동시에 하나의 writer만 허용한다.
- journal payload에는 player/ticket 정보가 포함될 수 있으므로 private application state로 취급하고 source repository나 일반 diagnostic artifact에 복사하지 않는다.
- file 복제, backup, compaction과 schema migration은 writer가 정지된 상태 또는 이후에 정의할 coordinated operation에서만 수행한다.

## Recovery And Retry

replay는 original event time과 identity를 사용한다. `RegisterPolicy`, ticket upsert, reserve, confirm, cancel과 assignment acknowledgment의 기존 idempotency/CAS contract가 process restart를 넘어 유지된다. active reservation은 저장된 TTL로 동일한 `ExpiresAt`을 다시 계산한다.

side-effect-free plan record는 engine state를 바꾸지 않고 audit와 authoritative proposal index에 복구된다. assignment delivery consumer는 같은 `assignmentID`와 `operationID`를 사용해 pending read model을 조회하고 acknowledgment를 반복해야 한다.

## Current Limits

- single replica와 한 journal writer만 지원한다.
- journal compaction/snapshot과 online backup은 아직 없다.
- Darwin/Linux 외 runtime은 지원하지 않는다.
- authentication, push delivery, multi-replica transport와 remote storage는 아직 없다.
- replay benchmark는 102/1002 event fixture를 실행하지만 numeric startup SLO는 P10 target hardware evidence 전까지 고정하지 않는다.

## Verification

```sh
go test ./internal/durable
go test -race ./internal/durable
go test ./internal/durable -run '^$' -bench '^BenchmarkOpenReplay$' -benchtime=1x
```
