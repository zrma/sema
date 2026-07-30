# Development Stages

Sema의 장기 개발은 세 개의 굵은 단계로 닫는다. 이 구분은 세부 P milestone을 대체하지 않고, 프로젝트가 무엇을 완료한 뒤 어디에서 멈추는지 설명하는 상위 lifecycle이다.

## Stage 1: Matcher Framework Core

- 범위: P0–P28.
- 상태: 완료.
- 결과: deterministic match composition, reservation/assignment lifecycle, evaluation lab, Flow simulation, quality/fairness/backfill evidence와 matcher V0 conformance.
- 종료 기준: algorithm-owned invariant가 executable fixture와 conformance gate로 고정되고, 남은 실제 MMR·traffic·region·role 수치는 consumer calibration 책임으로 분리된다.

## Stage 2: Reference Service Runtime

- 범위: P29–P31.
- 상태: 완료.
- 결과: PostgreSQL durable authority, provider-neutral OIDC, stateless service replica, reference client, multi-replica contention, workload, observability와 recovery acceptance.
- 종료 기준: 특정 게임이나 기존 배포 없이도 repository-owned evidence만으로 standalone service runtime의 compatibility, availability와 operational readiness를 반복 검증한다.

## Stage 3: Framework Contract Closure

- 범위: P32.
- 상태: 완료.
- 목표: 실제 서비스 연동이나 기존 매치메이커 교체가 아니라, 독립적인 공통 프레임워크가 장기간 유지할 public contract를 확정하고 저장소를 maintenance mode로 전환한다.
- 결과: P31 standard runtime, `p30-v0alpha2` first wire baseline, signed `v0.3.0`/`v0.4.0`/`v1.0.0` release, ADR 0033 stable service wire contract와 executable admission/matrix gate.
- 완료 증거:
  1. stable public surface와 제외 surface를 명시한다.
  2. compatibility, security exception, migration, rollback, numeric deprecation/support window와 maintenance owner를 승인한다.
  3. service wire를 포함한 tagged release 두 개 이상에서 supported previous/current client-service matrix를 실행한다.
  4. 위 contract와 matrix를 executable release gate에 연결한 뒤에만 `stable_admitted`를 변경한다.
  5. canonical local gate, public boundary gate, remote immutable target과 terminal CI를 확인한다.
  6. handoff/status/roadmap이 기능 개발 종료와 maintenance-only 진입을 같은 상태로 선언한다.

실제 game integration은 adoption evidence일 수 있지만 Stage 3의 입력이나 완료 조건이 아니다. 특정 provider, deployment, production traffic 또는 기존 system의 migration/cutover도 이 단계에 포함하지 않는다.

## Maintenance Mode

현재 lifecycle 상태다. Stage 3 완료로 Sema의 계획된 제품 개발은 종료했으며 활성 development milestone은 없다. 이후 기본 허용 범위는 다음과 같다.

- security, dependency와 toolchain maintenance.
- stable contract를 보존하는 bug, conformance와 regression 수정.
- 문서, release reproducibility와 supported compatibility matrix 유지.

새 matcher 기능, public surface 확장, provider-specific integration, production deployment program 또는 기존 service migration은 자동 후속 단계가 아니다. 별도 재개 결정과 새 milestone 없이 시작하지 않는다.
