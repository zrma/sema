# Release Admission

## Gate

`scripts/check-release-admission.sh <version>`은 다음을 순서대로 실행한다.

1. semantic version과 major channel 판정.
2. full Go/repository gate.
3. hardened container build/restart gate.
4. standard PostgreSQL workload/failure/recovery gate.
5. repeated V0 compatibility performance/recovery SLO gate.
6. release binary/checksum build gate.
7. repository publication boundary gate.

push, tag, release와 visibility 변경은 이 local admission과 별개의 외부-write 승인이다. public push 전 machine-local private-inventory gate도 별도로 필요하다.

## Alpha Admission

manifest의 `alpha_admitted: true`는 `v0.*` candidate가 위 gate를 통과하면 local release admission을 얻는다는 뜻이다. alpha API와 service는 compatibility를 약속하지 않으며 release note에 experimental boundary와 known limits를 유지한다.

## Stable Admission

major version 1 이상은 manifest의 exact `stable_admitted: true`가 추가로 필요하다. 현재 값은 false이며 다음 blocker가 남아 있다.

- stable 범위를 service wire만으로 할지 public Go alpha package까지 포함할지에 대한 명시적 승인.
- service wire를 포함한 tagged release 두 개 이상의 previous/current cross-version conformance.
- stable major/minor compatibility, security exception, numeric deprecation/support window와 support owner.

standard runtime, external TLS gateway fixture, reference client/workload, observability와 native PostgreSQL recovery acceptance는 완료되었다. P30 `v0alpha2`가 첫 wire baseline이고 기존 공개 tag에는 이 service가 없으므로 multi-release evidence를 가정하지 않는다. 실제 game integration은 adoption evidence이지 stable admission의 필수 조건이 아니다. blocker를 해결할 때는 compatibility/support decision과 executable cross-version evidence를 먼저 추가하고 마지막 change에서 admission flag를 바꾼다.

## Commands

```sh
scripts/check-release-admission.sh v0.1.0
scripts/check-release-admission.sh v1.0.0
```

첫 command는 모든 local gate를 실행한다. 두 번째 command는 현재 stable blocker를 안내하며 artifact build나 publication 전에 실패한다.
