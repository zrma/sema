# Wire Compatibility And Deprecation

## Current Baseline

표준 service wire는 experimental `v0alpha2`다. P30의 `sema-target-smoke` client/report와 lifecycle route를 `internal/wireconformance/testdata/p30-v0alpha2.json`의 첫 compatibility baseline으로 고정한다. 공개 `v0.1.0`과 `v0.2.0` tag는 이 service wire 이전이므로 아직 tagged service release 두 개를 교차 검증했다고 주장하지 않는다.

`sema-conformance`와 `sema.wire-conformance.v1`은 신규 standard 이름이다. `sema-target-smoke`와 `sema.target-smoke.v1`, `sema-target-server`는 P30 compatibility alias로 계속 보존한다.

## Alpha Policy

- additive optional field, endpoint 또는 bounded failure code 추가는 existing request/response decoding과 zero-value semantics를 깨지 않을 때만 같은 `v0alpha2`에 허용한다.
- required field 추가, field removal/rename/type change, route removal, status/failure 의미 변경은 breaking change다. 새 wire prefix와 migration document를 먼저 추가한다.
- predecessor route/command 제거는 자동 기한이 없다. 별도 deprecation decision, release note, migration/rollback limitation과 previous-client/current-service conformance가 없으면 제거하지 않는다.
- compatibility alias의 implementation 공유는 유지보수 방법일 뿐 evidence가 아니다. 고정 baseline, raw wire fixture와 command/report test를 함께 유지한다.
- diagnostic trace, metrics, audit payload와 workload report는 lifecycle wire API가 아니며 각 문서의 versioned schema를 따른다.

## Executable Evidence

- P30 baseline metadata가 `v0alpha2`, legacy command/report와 required lifecycle route/failure를 고정한다.
- legacy compatibility client가 current target handler에서 authentication, authorization, tenant isolation과 terminal lifecycle을 실행한다.
- external TLS gateway fixture가 HTTPS client → TLS termination → private HTTP listener 순서를 실행한다.
- standard current client는 PostgreSQL/OIDC fixture에서 같은 lifecycle과 expanded report를 실행한다.
- `scripts/check-wire-compatibility-matrix.sh <previous-tag> <current-tag>`는 서로 다른 annotated semver tag를 exact commit으로 해석하고 각 source의 reference client와 loopback wire fixture를 별도 빌드해 previous client → current service와 current client → previous service를 실행한다.

이 evidence는 현재 source가 첫 baseline을 보존한다는 뜻이다. 과거 tagged service binary와 current client를 교차 실행한 multi-release evidence는 아니다.

matrix의 `--self-test`는 current source를 양쪽 역할로 실행해 process orchestration, redaction과 report schema만 검증한다. 출력 schema도 `sema.wire-compatibility-matrix-self-test.v1`로 분리하며 tagged multi-release evidence로 계산하지 않는다. 정상 matrix report는 tag/version, exact same-repository commit과 방향별 boolean만 남기고 token, endpoint와 raw payload를 보존하지 않는다.

## Stable Admission Requirements

stable 전환은 다음을 모두 요구한다.

1. stable 범위가 wire service만인지 public Go package까지인지 명시적으로 선택한다.
2. service wire를 포함한 tagged release 두 개 이상에서 previous client → current service와 current client → supported previous service matrix를 실행한다.
3. stable major/minor compatibility, security exception, 최소 deprecation/support window와 support owner를 승인한다.
4. breaking migration, rollback limitation과 end-of-support 신호를 executable release gate에 연결한다.
5. operational workload, failure, observability와 recovery gate가 계속 통과한다.

현재 repository는 첫 baseline과 policy를 갖췄지만 1~4의 stable 약속을 시작하지 않았다. 따라서 `stable_admitted: false`가 정확한 결정이다. 실제 game integration은 이 판단의 필수 조건이 아니다.
