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

tag workflow는 local admission 뒤에 manifest의 exact previous/current pair를 `scripts/check-wire-compatibility-release.sh`로 실행한다. 이 post-tag gate는 두 annotated tag object가 모두 존재해야 하므로 pre-tag local admission과 구분한다. 정상 matrix JSON은 release artifact와 `SHA256SUMS`에 포함하며 self-test는 이 evidence를 대신하지 않는다.

## Alpha Admission

manifest의 `alpha_admitted: true`는 `v0.*` candidate가 위 gate를 통과하면 local release admission을 얻는다는 뜻이다. alpha API와 service는 compatibility를 약속하지 않으며 release note에 experimental boundary와 known limits를 유지한다.

## Stable Admission

major version 1 이상은 manifest의 exact `stable_admitted: true`가 추가로 필요하다. 현재 값은 false이며 다음 blocker가 남아 있다.

- migration, rollback limitation과 end-of-support 신호의 executable release gate 연결.
- stable release note와 exact predecessor/current matrix pair.

ADR 0033은 stable 범위를 HTTP `/v1` service wire로 한정하고 Go `alpha`를 제외했다. 같은 major의 additive compatibility, critical security exception, 180일/2개 minor deprecation window와 repository maintainer ownership을 승인했다. `v0.3.0`과 `v0.4.0` tagged release의 양방향 matrix도 release asset으로 검증했다. standard runtime, external TLS gateway fixture, reference client/workload, observability와 native PostgreSQL recovery acceptance는 계속 canonical gate가 소유한다. 실제 game integration은 adoption evidence이지 stable admission의 필수 조건이 아니다. 남은 gate를 연결한 마지막 change에서만 admission flag를 바꾼다.

## Commands

```sh
scripts/check-release-admission.sh v0.1.0
scripts/check-release-admission.sh v1.0.0
```

첫 command는 모든 local gate를 실행한다. 두 번째 command는 현재 stable blocker를 안내하며 artifact build나 publication 전에 실패한다.
