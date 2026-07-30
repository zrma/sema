# ADR 0032: Stable Admission Deferred After P31

## Status

Accepted

## Context

P31은 standard runtime, current wire conformance, multi-replica failure matrix, workload/resource profile, observability와 native PostgreSQL recovery acceptance를 완료했다. stable admission은 이 operational evidence 외에도 장기간 유지할 public compatibility surface와 deprecation/support 약속이 필요하다.

공개 `v0.1.0`과 `v0.2.0` tag에는 `v0alpha2` service wire가 없다. P30 wire는 첫 compatibility baseline으로 고정할 수 있지만 두 tagged service release 간 교차 호환성을 이미 입증했다고 볼 수 없다. stable 범위를 service wire만으로 할지 public Go alpha package까지 포함할지도 아직 승인되지 않았다.

## Decision

- P31 product-readiness milestone은 완료하되 `release.stable_admitted`는 `false`로 유지한다.
- `p30-v0alpha2`를 첫 wire compatibility baseline으로 보존하고 breaking alpha change는 새 prefix/migration decision을 요구한다.
- P30 command/report alias는 별도 deprecation decision과 cross-version evidence 전까지 제거하지 않는다.
- stable admission은 surface scope, tagged multi-version matrix, numeric deprecation/support window와 support owner를 명시적으로 승인하는 후속 milestone로 분리한다.
- 실제 game traffic 또는 external consumer integration은 stable gate의 필수 조건으로 추가하지 않는다.

## Consequences

repository는 PoC 형태의 V0를 벗어난 deployable standard service와 반복 가능한 operational evidence를 갖지만 v1 compatibility를 과장하지 않는다. alpha release와 실험은 계속 가능하며 첫 service release부터 다음 cross-version matrix의 immutable 입력이 생긴다.

## Rejected Alternatives

- operational gate 통과만으로 즉시 stable 선언: 유지할 surface와 지원 기간이 없다.
- 기존 pre-service tag를 multi-version service evidence로 계산: 해당 wire/binary가 존재하지 않는다.
- 실제 게임 연동 전까지 P31 자체를 미완료로 유지: standalone product scope와 repository-owned readiness decision을 다시 consumer deployment에 종속시킨다.

## Revisit When

- service wire를 포함한 tagged release가 두 개 이상 존재한다.
- stable surface와 deprecation/support ownership을 승인할 수 있다.
- cross-version matrix가 supported predecessor/current 조합을 모두 통과한다.
