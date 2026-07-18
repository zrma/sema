# Operations Runbook

## Supported Deployment Envelope

현재 service deployment baseline은 local POSIX volume을 소유한 single Linux container와 정확히 하나의 journal writer다. replica 수는 1이며 같은 journal volume을 두 process나 두 container에 동시에 mount하지 않는다. horizontal coordination, external database와 journal compaction은 지원하지 않는다.

HTTP API에는 built-in authentication과 TLS가 없다. 기본 image는 container loopback만 listen한다. `deploy/compose.yaml`은 명시적 unsafe flag를 사용하지만 host의 `127.0.0.1`에만 port를 publish한다. remote client가 필요하면 승인된 authentication/TLS/rate-limit gateway 뒤의 private network에 두고 direct service port를 외부에 publish하지 않는다.

## Container Contract

- runtime identity: numeric non-root `65532:65532`.
- root filesystem: read-only 가능.
- writable paths: `/var/lib/sema` durable volume과 `/tmp` bounded tmpfs.
- Linux capabilities: none; privilege escalation disabled.
- journal: `/var/lib/sema/sema.journal`, mode `0600`.
- health: image `HEALTHCHECK`가 loopback `/readyz`를 조회한다.
- shutdown: `SIGTERM` 뒤 최대 10초 graceful HTTP drain; deployment stop grace는 15초 이상.

image의 builder base는 exact Go/Alpine multi-platform digest로 고정하고 runtime은 `scratch`다. base digest 변경은 container gate와 publication scan을 같은 change에서 다시 통과해야 한다.

## Local Deployment Example

```sh
docker compose -f deploy/compose.yaml up --build -d
docker compose -f deploy/compose.yaml ps
```

service는 host `127.0.0.1:8080`에서만 접근한다. 종료는 다음 순서다.

```sh
docker compose -f deploy/compose.yaml stop
docker compose -f deploy/compose.yaml down
```

`down`은 named volume을 제거하지 않는다. `down --volumes`는 journal을 삭제하므로 명시적인 데이터 폐기 작업에서만 사용한다.

## Health And Telemetry

- `/livez`: process가 HTTP를 처리한다.
- `/readyz`: journal runtime이 open이고 poisoned/closed 상태가 아니다.
- `/metrics`: process-local request counter와 latency histogram.
- `/v0alpha1/audit`: redacted durable decision summary.

readiness failure, non-zero restart loop 또는 metric의 5xx 증가는 새 mutation을 중단하고 journal owner를 하나로 고정한 뒤 조사한다. trace/audit exporter는 raw identity를 노출하지 않지만 journal file과 container stderr는 private application data로 취급한다.

## Backup

online backup은 지원하지 않는다. 일관된 backup은 다음 순서를 지킨다.

1. ingress/producer를 quiesce하고 in-flight request가 끝날 때까지 기다린다.
2. container를 graceful stop해 writer lock을 해제한다.
3. stopped volume의 journal을 operator-owned encrypted backup에 snapshot한다.
4. backup의 size와 cryptographic digest를 private inventory에 기록한다.
5. 원본 container를 다시 시작하고 readiness를 확인한다.

backup, digest, host/volume identity와 raw journal은 public repository나 일반 CI artifact에 기록하지 않는다.

## Restore And Recovery

1. producer를 중지하고 service container를 stop한다.
2. current journal을 private incident archive로 보존한다.
3. 같은 schema와 reservation TTL로 만들어진 verified backup을 stopped volume에 복원한다.
4. file owner를 runtime identity, permission을 `0600`으로 유지한다.
5. service를 한 replica로 시작하고 readiness, audit sequence와 expected assignment read model을 확인한다.
6. producer는 기존 idempotency ID로 uncertain operation을 retry한다.

newline 없는 마지막 record는 startup에서 제거된다. newline이 완성된 record의 checksum/schema/sequence 오류는 자동 수리하지 않고 startup을 실패시킨다. 손상된 complete record를 삭제하거나 건너뛰지 말고 known-good backup으로 복원한다.

## Failure Triage

| Symptom | Expected action |
|---|---|
| second-writer lock failure | duplicate replica를 중지하고 volume owner를 하나로 만든다 |
| reservation TTL mismatch | original configured TTL로 시작하고 configuration drift를 수정한다 |
| journal checksum/sequence failure | writer를 중지하고 known-good offline backup으로 복원한다 |
| readiness 503 after I/O failure | mutation을 차단하고 disk health/capacity와 private logs를 조사한다 |
| restart 후 client timeout | 같은 snapshot/reservation/assignment/operation ID로 poll 또는 retry한다 |
| volume capacity growth | 새 ingestion을 제한하고 offline backup을 만든다; ad-hoc compaction은 하지 않는다 |

## Upgrade And Rollback

upgrade 전 offline backup과 full release gate를 통과한다. replica 1을 stop하고 새 image digest로 교체한 뒤 readiness/replay를 확인한다. 현재 `sema-journal-v1` schema를 바꾸는 release는 별도 migration decision 없이는 허용하지 않는다.

rollback은 target binary가 existing journal schema와 configuration을 읽을 수 있을 때만 수행한다. startup 검증이 실패하면 반복 재시작하지 말고 이전 image와 pre-upgrade backup을 사용해 stopped-volume restore를 수행한다.

## Validation

```sh
scripts/check-container.sh
go run ./cmd/sema-ops-check -cycles 100 -tickets-per-cycle 20 -concurrency 16 -timeout 2m
```

첫 command는 image user/version, in-image lifecycle validation, read-only/capability-reduced startup과 volume-backed restart를 확인한다. 두 번째 command는 외부 state를 건드리지 않는 bounded local soak 예다.

## Target PostgreSQL Cutover Rehearsal

위 container 절차는 현재 V0 single-writer runtime의 운영 계약이다. target PostgreSQL runtime을 remote traffic에 열기 전 local cutover evidence는 다음 disposable gate가 소유한다.

```sh
scripts/check-postgres.sh
```

이 gate는 pinned test PostgreSQL 안에서 isolated schema만 사용해 stopped V0 fixture import, custom-format logical backup, target schema 삭제, restore와 semantic manifest 비교를 수행한다. 복원된 import completion marker와 terminal assignment를 읽은 뒤 target schema를 다시 폐기하고 original V0 journal을 기존 TTL로 재기동한다. journal open/close 전후 source digest가 달라지면 실패한다.

private manifest에는 source digest/record count, repository version, resource/audit digest, metadata/scope/operation authority digest와 repository table별 row count만 mode `0600`으로 임시 저장한다. DSN, raw resource, journal path, dump와 environment identity는 tracked 문서나 일반 CI artifact에 보존하지 않는다.

이 local logical restore는 production backup 승인이 아니다. 실제 provider의 encryption, retention, PITR, access control, restore location과 RPO/RTO는 provider 선택 뒤 별도 rehearsal로 검증한다. target writer의 첫 mutation 뒤에는 V0 rollback을 금지하며 compatible target binary와 target PostgreSQL backup으로만 되돌린다.
