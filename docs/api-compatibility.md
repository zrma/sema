# API Compatibility And Migration

## Current Stage

public import path는 `github.com/zrma/sema/alpha`, API marker는 `v0alpha5`다. source와 example은 공개되어 있지만 production stability와 semantic-version compatibility를 아직 약속하지 않는다.

`internal/` package는 계속 non-public implementation detail이며 외부 consumer가 직접 import할 수 없다.

V0 development/reference service는 `v0alpha1`, 표준 authenticated PostgreSQL service는 stable `/v1` wire와 `/v0alpha2` compatibility marker를 사용한다. Go alpha와 service wire는 독립적인 compatibility surface이며 한쪽 변경이 다른 쪽 version bump를 자동으로 뜻하지 않는다. 표준 wire의 baseline과 deprecation rule은 `docs/wire-compatibility.md`와 ADR 0033이 소유한다.

## Alpha Change Policy

- additive field/function도 consumer compile behavior와 zero-value semantics를 검토한다.
- field rename/removal, type change, objective ordering 또는 failure-code 의미 변경은 breaking change다.
- breaking alpha change는 `APIVersion`을 올리고 `docs/migrations/`에 old-to-new mapping을 추가한다.
- example consumer, public-package test와 release smoke를 같은 change에서 갱신한다.
- diagnostic JSON을 wire contract로 사용하지 않는다. wire protocol이 필요하면 별도 versioned schema/ADR을 만든다.

Go module이 v1 이전인 동안 semantic versioning은 breaking minor release를 허용하지만, repository는 silent break를 허용하지 않는다. release note와 migration document가 break를 명시해야 한다.

## Stable API Gate

public Go package를 stable surface에 포함하려면 다음을 모두 만족하기 전에는 root/stable package를 선언하지 않는다.

- stable 범위와 support owner를 명시적으로 승인한다.
- 최소 두 tagged release cycle에서 reference consumer와 compatibility fixture가 필요한 type/behavior를 관찰한다.
- deterministic replay, failure behavior와 migration test가 있다.
- persistence/service boundary와 분리된 composition responsibility가 유지된다.
- support 범위, deprecation window와 numeric compatibility policy가 문서화된다.

ADR 0033은 stable 범위를 HTTP `/v1` service wire로 한정했다. public Go `alpha` package는 계속 experimental이며 stable Go package 승격에는 위 evidence를 갖춘 별도 milestone이 필요하다. external game consumer는 service stable evidence의 필수 조건이 아니다.

## Migration Layout

현재까지 breaking alpha change는 다음 migration 문서가 소유한다.

```text
docs/migrations/v0alpha1-to-v0alpha2.md
docs/migrations/v0alpha2-to-v0alpha3.md
docs/migrations/v0alpha3-to-v0alpha4.md
docs/migrations/v0alpha4-to-v0alpha5.md
docs/migrations/v0alpha2-to-v1.md
```

문서는 변경된 marker/field, behavior difference, before/after code와 rollback limitation을 포함한다. 다음 breaking change도 같은 directory와 old-to-new naming을 사용한다.
