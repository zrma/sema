# Release Admission

## Gate

`scripts/check-release-admission.sh <version>`은 다음을 순서대로 실행한다.

1. semantic version과 major channel 판정.
2. latency-sensitive standard PostgreSQL workload/failure/recovery gate.
3. repeated V0 compatibility performance/recovery SLO gate.
4. full Go/repository gate.
5. hardened container build/restart gate.
6. release binary/checksum build gate.
7. repository publication boundary gate.

latency-sensitive gate를 compile/benchmark와 container build보다 먼저 실행해 같은 machine의 선행 부하를 reference regression으로 오인하지 않게 한다. 순서만 조정하며 profile, 반복 횟수와 numeric budget은 완화하지 않는다.

push, tag, release와 visibility 변경은 이 local admission과 별개의 외부-write 승인이다. public push 전 machine-local private-inventory gate도 별도로 필요하다.

tag workflow는 local admission 뒤에 manifest의 exact previous/current pair를 `scripts/check-wire-compatibility-release.sh`로 실행한다. 이 post-tag gate는 두 annotated tag object가 모두 존재해야 하므로 pre-tag local admission과 구분한다. 정상 matrix JSON은 release artifact와 `SHA256SUMS`에 포함하며 self-test는 이 evidence를 대신하지 않는다.

## Alpha Admission

manifest의 `alpha_admitted: false`는 stable line에서 새 `v0.*` publication을 닫는다는 뜻이다. 기존 `v0.3.0`과 `v0.4.0`은 immutable compatibility evidence로 남는다. public Go `alpha` package는 source에서 experimental로 유지되지만 이후 repository release는 stable service-wire tag와 version-matched release note를 사용한다.

## Stable Admission

major version 1 이상은 manifest의 exact `stable_admitted: true`가 추가로 필요하다. 현재 값은 true다. gate는 다음 machine-readable declaration을 exact match로 요구한다.

- stable surface `service_wire_v1`과 repository maintainer owner.
- immediate previous stable minor 1개, 최소 180일 support와 2개 minor deprecation window.
- `/v0alpha2` alias의 `supported` 상태와 `not_scheduled` end-of-support 신호.
- current release version과 일치하는 stable scope, migration/rollback와 known-limit release note.
- manifest의 exact predecessor/current pair를 실행하는 post-tag matrix.

ADR 0033은 stable 범위를 HTTP `/v1` service wire로 한정하고 Go `alpha`를 제외했다. 같은 major의 additive compatibility, critical security exception, 180일/2개 minor deprecation window와 repository maintainer ownership을 승인했다. `v0.3.0`과 `v0.4.0` tagged release의 양방향 matrix도 release asset으로 검증했다. standard runtime, external TLS gateway fixture, reference client/workload, observability와 native PostgreSQL recovery acceptance는 계속 canonical gate가 소유한다. 실제 game integration은 adoption evidence이지 stable admission의 필수 조건이 아니다.

첫 stable baseline인 `v1.0.0`은 local admission과 같은-target Release workflow를 통과했고, `v0.4.0 ↔ v1.0.0` 양방향 matrix를 checksummed release asset으로 게시했다. 이 출고 완료는 stable surface를 넓히지 않으며 이후 저장소 lifecycle은 maintenance mode다.

## Commands

```sh
scripts/check-release-admission.sh v0.1.0
scripts/check-release-admission.sh v1.0.0
```

첫 command는 alpha publication이 닫혀 있어 거부된다. 두 번째 command는 full local admission을 실행하며, 실제 tag workflow가 `v0.4.0 → v1.0.0` matrix를 추가로 실행한다.
