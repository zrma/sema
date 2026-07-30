# Service Wire Migration: v0alpha2 To v1

## Scope

이 migration은 authenticated HTTP service wire만 다룬다. public Go `alpha` package와 V0 `/v0alpha1` development/reference service는 `v1` stable service wire 약속에 포함되지 않는다.

## Route Mapping

payload, permission, status, failure code와 lifecycle semantics는 유지하고 prefix와 response envelope marker만 바뀐다.

| v0alpha2 | v1 |
|---|---|
| `/v0alpha2/policies/...` | `/v1/policies/...` |
| `/v0alpha2/match-tickets/...` | `/v1/match-tickets/...` |
| `/v0alpha2/backfill-tickets/...` | `/v1/backfill-tickets/...` |
| `/v0alpha2/planning-runs/...` | `/v1/planning-runs/...` |
| `/v0alpha2/reservations/...` | `/v1/reservations/...` |
| `/v0alpha2/assignments/...` | `/v1/assignments/...` |
| response `api_version: "v0alpha2"` | response `api_version: "v1"` |

`/livez`, `/readyz`와 `/metrics`는 version prefix 밖의 operational endpoint로 유지한다.

## Client Migration

신규 client는 `/v1`을 사용하고 response의 `api_version`이 `v1`인지 확인한다.

```sh
sema-conformance
```

`sema-conformance`의 기본 prefix는 `/v1`이다. compatibility 검사나 bounded rollback이 필요한 client만 다음처럼 alpha route를 명시한다.

```sh
sema-conformance -api-version v0alpha2
```

P30 command/report compatibility를 위한 `sema-target-smoke`는 `/v0alpha2`를 기본으로 유지한다.

## Compatibility And Support

두 prefix는 같은 durable authority와 resource identity를 공유한다. 한 prefix로 생성한 resource를 다른 prefix로 읽어도 같은 lifecycle state를 반환하며 response marker만 요청 prefix에 맞춘다.

`/v0alpha2`는 `v1.0.0` 이후 최소 180일과 두 번의 후속 minor release 중 더 긴 기간 동안 지원한다. 제거 전 release note와 machine-readable admission input에 end-of-support version/date, migration evidence와 rollback limitation을 기록한다. support owner는 repository maintainers다.

## Rollback

client는 지원 기간 안에 endpoint prefix를 `/v0alpha2`로 되돌릴 수 있다. durable payload, repository schema 또는 resource ID를 변환하지 않으므로 server data rollback은 필요하지 않다.

다만 `/v1` 이후 추가된 additive field나 endpoint를 사용하는 client는 `/v0alpha2` rollback 전에 해당 사용을 중지해야 한다. future security exception이 `/v0alpha2`를 조기 제한할 경우 advisory가 대체 route, migration과 rollback limitation을 명시한다.

## Out Of Scope

이 migration은 production traffic cutover, 기존 matcher/database 이전, provider 선택, token acquisition 또는 deployment-specific TLS 구성을 실행하지 않는다.
