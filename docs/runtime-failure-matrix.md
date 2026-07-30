# Runtime Failure Matrix

## Purpose

`cmd/sema-runtime-matrix`는 표준 PostgreSQL runtime의 shared-authority behavior를 disposable schema에서 검증한다. 두 stateless replica는 별도 PostgreSQL pool과 HTTP handler를 가지며 같은 repository schema, cursor key, reservation TTL과 authenticated tenant를 사용한다.

OIDC discovery/JWT claim 검증은 `docs/wire-conformance.md`의 ephemeral provider fixture가 별도로 소유한다. 이 matrix는 같은 authenticated target handler 뒤에서 PostgreSQL contention, replica restart와 dependency outage/recovery가 lifecycle authority를 깨뜨리지 않는지 격리해 측정한다.

## Scenarios

| Scenario | Required evidence |
|---|---|
| reservation contention | 같은 proposal을 서로 다른 reservation ID로 두 replica가 동시에 claim하고 정확히 하나만 active winner가 된다 |
| peer completion | winner가 아닌 replica가 confirm하고 다른 replica가 acknowledge해 두 replica가 같은 terminal assignment/storage version을 읽는다 |
| replica restart | 한 replica의 listener와 pool을 닫아 새 instance로 열어도 terminal assignment를 동일하게 읽는다 |
| PostgreSQL outage | controllable TCP fault proxy가 active connection을 끊고 신규 연결을 거부할 때 두 replica `/readyz`는 503, `/livez`는 200이다 |
| request failure | outage 중 assignment read는 retryable `503 Unavailable`이며 raw database detail을 반환하지 않는다 |
| PostgreSQL recovery | proxy가 연결을 다시 허용하면 두 replica readiness와 terminal assignment agreement가 복구된다 |

fault proxy는 PostgreSQL container나 external database를 수정하지 않는다. isolated service pool의 network path만 끊고 복구해 connection loss를 재현한다. matrix가 만든 schema는 성공/실패와 무관하게 제거한다.

## Sanitized Report

성공 report schema는 `sema.runtime-failure-matrix.v1`이다.

```json
{
  "schema": "sema.runtime-failure-matrix.v1",
  "replica_count": 2,
  "reservation_winner_count": 1,
  "reservation_conflict_count": 1,
  "terminal_agreement": true,
  "replica_restart_recovered": true,
  "dependency_readiness_failed_closed": true,
  "dependency_request_failed_retryable": true,
  "liveness_maintained": true,
  "dependency_recovery_complete": true
}
```

report에는 DSN, address, schema, token, tenant/resource ID, request/response payload나 raw error가 없다. command failure도 infrastructure detail을 stdout/stderr에 출력하지 않는다.

## Execution

일반적으로 pinned PostgreSQL container와 import/recovery rehearsal을 함께 소유하는 gate를 실행한다.

```sh
scripts/check-postgres.sh
```

외부 test-only database로 matrix만 실행할 때는 disposable database credential을 environment로 전달한다.

```sh
SEMA_POSTGRES_TEST_DSN='<test-dsn>' go run ./cmd/sema-runtime-matrix
```

shared/user database나 production data에 실행하지 않는다. matrix는 임의 schema를 생성·삭제할 권한이 필요하다.

## Boundary

이 evidence는 한 PostgreSQL primary 안의 stateless horizontal replica와 transient dependency loss를 다룬다. PostgreSQL failover product, cross-region primary, backup/PITR, long-duration load, network partition의 모든 형태나 numeric RTO/SLA를 주장하지 않는다. workload admission/pool/timeout과 backup/PITR evidence는 P31의 다음 단계다.
