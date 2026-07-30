# Wire Compatibility And Deprecation

## Current Baseline

표준 stable service wire는 `/v1`이다. P30의 `sema-target-smoke` client/report와 `/v0alpha2` lifecycle route를 `internal/wireconformance/testdata/p30-v0alpha2.json`의 첫 compatibility baseline으로 고정하고 ADR 0033의 지원 기간 동안 alias로 유지한다. `v0.3.0`과 `v0.4.0`이 이 service wire를 포함한 첫 두 tagged release이며 양방향 matrix를 release asset으로 보존한다.

`sema-conformance`와 `sema.wire-conformance.v1`은 `/v1` standard 이름이다. `sema-target-smoke`와 `sema.target-smoke.v1`, `sema-target-server`는 P30 compatibility alias로 계속 보존한다. current client가 supported alpha predecessor service를 검사할 때는 `-api-version v0alpha2`를 명시한다.

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
- `scripts/check-wire-compatibility-release.sh <current-tag> <report-directory>`는 manifest의 exact pair만 허용한다. 첫 pair인 service distribution baseline `v0.3.0`과 matrix-enabled `v0.4.0`은 각 annotated tag의 exact source를 양방향으로 실행했고 matrix JSON을 `v0.4.0` checksum 대상 release asset으로 보존했다.

matrix의 `--self-test`는 current source를 양쪽 역할로 실행해 process orchestration, redaction과 report schema만 검증한다. 출력 schema도 `sema.wire-compatibility-matrix-self-test.v1`로 분리하며 tagged multi-release evidence로 계산하지 않는다. 정상 matrix report는 tag/version, exact same-repository commit과 방향별 boolean만 남기고 token, endpoint와 raw payload를 보존하지 않는다. tag workflow는 이 JSON을 checksum 대상 release asset으로 보존한다.

## Stable Admission Requirements

stable 전환은 다음을 모두 요구한다.

1. stable 범위를 HTTP `/v1` service wire로 한정하고 제외 surface를 명시한다.
2. service wire를 포함한 tagged release 두 개 이상에서 previous client → current service와 current client → supported previous service matrix를 실행한다.
3. 같은 major의 additive compatibility, security exception, 최소 deprecation/support window와 support owner를 승인한다.
4. breaking migration, rollback limitation과 end-of-support 신호를 executable release gate에 연결한다.
5. operational workload, failure, observability와 recovery gate가 계속 통과한다.

ADR 0033은 stable 범위를 `/v1` wire로 한정하고 repository maintainers를 owner로 정했다. 최신 stable minor와 바로 이전 minor는 successor release 후 최소 180일 동안 지원하며, deprecated route/field/command는 두 번의 후속 minor와 180일 중 더 긴 기간 동안 보존한다. critical authentication, tenant isolation 또는 data-integrity security defect만 advisory, migration, rollback limitation, regression evidence와 end-of-support 신호를 갖춘 조기 예외가 될 수 있다.

`/v0alpha2`는 같은 durable state를 사용하는 compatibility alias다. `v1.0.0` 이후 최소 180일과 두 번의 후속 minor release 중 더 긴 기간 동안 유지한다. 현재 machine-readable 신호는 `supported`와 `not_scheduled`이며 successor cadence가 실제 removal floor를 결정하면 release note와 manifest를 함께 갱신한다. 실제 game integration은 stable 판단의 필수 조건이 아니다. migration/release gate 연결이 완료되어 manifest의 `stable_admitted`는 `true`다.
