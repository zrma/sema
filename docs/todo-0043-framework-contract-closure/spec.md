# P32 Framework Contract Closure And Maintenance Handoff Spec

- Status: Completed — Maintenance Mode

## Objective

Sema의 세 번째이자 마지막 계획 개발 단계를 닫는다. P31 standard service를 실제 게임에 연동하거나 기존 매치메이커를 대체하지 않고, 독립적인 공통 프레임워크가 유지할 stable contract와 cross-version evidence를 완성한 뒤 저장소를 maintenance mode로 전환한다.

## Fixed Inputs

- `docs/development-stages.md`의 Stage 1과 Stage 2를 이어 P32까지 완료되었고 active development milestone은 없다.
- Sema는 standalone general-purpose matchmaking service이며 실제 consumer, production traffic 또는 predecessor deployment를 전제하지 않는다.
- PostgreSQL primary가 durable authority이고 service replica는 stateless하다. provider-neutral OIDC와 external TLS ownership을 유지한다.
- `p30-v0alpha2`가 첫 service wire compatibility baseline이다. 기존 공개 `v0.1.0`과 `v0.2.0`은 이 wire를 포함하지 않는다.
- public Go package는 alpha이고 service `/v1`만 stable surface다. release gate의 contract blocker는 해소되어 `release.stable_admitted`는 `true`다.
- public Go `alpha` package와 service wire는 독립적인 compatibility surface다.

## Implementation Sequence

- [x] 세 단계의 상위 lifecycle과 Stage 3 종료 후 maintenance-only 목표를 repository 문서에 고정한다.
- [x] service, migration과 reference client alpha release artifact 및 tagged-source wire fixture baseline을 제공한다.
- [x] stable 범위를 HTTP service wire `/v1`로 한정하고 public Go `alpha` package를 제외한다.
- [x] major/minor compatibility, security exception, numeric deprecation/support window와 repository maintainer ownership을 ADR 0033으로 고정한다.
- [x] immutable tagged service release를 입력으로 previous client → current service와 current client → supported previous service matrix를 실행하는 repository-owned gate를 구현한다.
- [x] manifest의 exact release pair와 tag workflow를 연결하고 bounded matrix JSON을 checksum 대상 release asset으로 보존한다.
- [x] service wire를 포함한 `v0.3.0`과 `v0.4.0`을 별도 publication 승인 아래 출고하고 양방향 matrix artifact를 보존한다.
- [x] migration, rollback limitation과 `/v0alpha2` supported/not-scheduled end-of-support 신호를 compatibility/release gate에 연결한다.
- [x] canonical local, container, PostgreSQL workload/failure/recovery와 public boundary gate를 통과한 뒤 stable admission을 별도 logical change로 전환한다.
- [x] stable release의 remote tag, artifact/checksum, same-target CI를 검증한다.
- [x] handoff/status/roadmap을 maintenance mode로 닫고 활성 development milestone을 남기지 않는다.

## Completion Evidence

- signed annotated `v1.0.0` tag는 commit `383071a4849994f5768a4ba7f9867e61b8b8d5c1`을 가리킨다.
- GitHub Release workflow run `30559193689`가 같은 target에서 terminal success로 끝났다.
- release의 22개 asset은 `SHA256SUMS`를 통과했고 네 public command의 darwin/arm64 artifact가 `v1.0.0`을 보고했다.
- `wire-compatibility-matrix.json`은 `v0.4.0` commit `c8c4b35ffaa090b7bebe9da8a8f511ced668d3e6`과 `v1.0.0` 사이의 두 client/service 방향을 모두 통과했다.
- repository 밖의 임시 consumer가 `github.com/zrma/sema/examples/compose@v1.0.0`을 내려받아 실행했다.

## Acceptance

- stable surface, version compatibility와 support/deprecation 책임이 모호하지 않다.
- supported tagged predecessor/current 조합을 양방향으로 실행하며 current-source fixture를 multi-release evidence로 대신하지 않는다.
- stable release gate가 scope, matrix, migration/support policy와 기존 operational gate를 모두 기계적으로 확인한다.
- 실제 game integration, production traffic 또는 private deployment inventory 없이 repository-owned fixture로 결과를 재현한다.
- `stable_admitted: true`는 모든 blocker가 제거된 마지막 change에서만 설정된다.
- Stage 3 완료 뒤 문서가 Sema를 기능 개발 완료·maintenance-only 상태로 일관되게 설명한다.

## Out Of Scope

- 실제 게임 backend 연동, player login, production MMR/rating calibration.
- 기존 매치메이커, queue, database 또는 traffic의 migration/cutover.
- 특정 OIDC provider, Kubernetes distribution, managed PostgreSQL 제품 또는 private deployment의 표준화.
- production-calibrated SLA, cross-region multi-primary, broker, Redis, streaming/event platform과 public SDK 확장.
- evidence 없이 stable surface를 넓히거나 P31 operational readiness만으로 v1을 선언하는 작업.

## Decision And Publication Gates

stable surface와 support/deprecation 약속은 장기 compatibility 책임을 바꾸므로 명시적 승인이 필요하다. push 권한은 change publication에만 적용하며 tag, release와 `stable_admitted` 전환은 각각 별도 출고 경계로 검증한다. public 출고 전에는 repository publication gate와 권한 있는 machine-local private-inventory gate를 모두 통과한다.

## Stop Condition

P32 acceptance를 모두 충족하면 계획된 제품 개발을 종료한다. 이후 작업은 `docs/development-stages.md`의 Maintenance Mode 범위로 제한하며, 새 기능·integration·deployment program은 별도 재개 결정과 새 milestone 없이는 시작하지 않는다.
