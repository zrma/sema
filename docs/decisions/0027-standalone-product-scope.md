# ADR 0027: Standalone Product Scope

- Status: Accepted
- Date: 2026-07-23

## Context

Sema는 기존에 운영 중인 매치메이커를 교체하거나 실제 게임 트래픽을 이전하는 프로젝트가 아니다. 범용 매치메이킹 시스템을 알고리즘, lifecycle, persistence와 service boundary부터 새로 구현하는 독립형 open-source product다.

저장소 안의 `cmd/sema-server`와 `sema-journal-v1`은 초기 구현을 검증한 V0 reference runtime이다. PostgreSQL/OIDC 기반 `cmd/sema-target-server`는 그 다음 제품형 runtime이다. 이전 문서에서 이 관계를 `production traffic cutover`로 표현하면서 실제 predecessor deployment와 consumer traffic이 존재하는 것처럼 읽힐 수 있었다.

## Decision

- Sema의 product scope는 특정 게임, 기존 배포 또는 migration program에 종속되지 않는 standalone general-purpose matchmaking service다.
- 게임 backend와 다른 caller는 Sema API의 잠재 consumer이지만, 저장소 milestone은 실제 consumer나 production traffic의 존재를 전제로 하지 않는다.
- repository-owned reference client, deterministic workload, compatibility fixture와 failure/recovery test가 제품 readiness의 기본 evidence다. 실제 consumer integration은 별도 adoption evidence이지 stable release의 필수 선행 조건이 아니다.
- `cmd/sema-server`와 V0 journal은 development/reference 및 optional import compatibility surface다. 이를 제품 기본 runtime에서 내리는 작업은 기존 운영 시스템의 cutover가 아니라 repository runtime surface의 정상적인 승격과 정리다.
- V0 import, backup/restore와 rollback rehearsal은 사용자가 해당 format에서 이전할 때 적용할 수 있는 migration/recovery capability다. 이 capability의 존재가 실제 deployed predecessor를 의미하지 않는다.
- OIDC/TLS/PostgreSQL reference deployment acceptance는 provider-neutral runtime contract가 실제 환경에서도 조립됨을 검증한다. 이를 실제 게임 트래픽 전환이나 production activation으로 표현하지 않는다.
- `cutover`는 특정 deployment operator가 실제 write authority를 바꾸는 절차에만 사용한다. repository roadmap과 현재 상태는 `runtime promotion`, `reference deployment acceptance`, `product readiness` 용어를 사용한다.
- P30은 authenticated PostgreSQL service runtime과 reference deployment acceptance를 완료한 milestone으로 닫는다. 다음 program은 P31 Service Product Readiness다.

## Consequences

- 현재 구현과 migration/recovery fixture는 폐기하지 않는다. 바뀌는 것은 제품 정체성, milestone 완료 기준과 default runtime의 해석이다.
- provider-specific identity mapping, endpoint와 credential은 deployment-owned이며 공개 저장소에는 provider-neutral contract와 redacted acceptance만 남긴다.
- 안정화 작업은 실제 게임 연동을 기다리지 않고 repository-owned client, conformance corpus, multi-replica contention, load/failure와 recovery evidence로 진행할 수 있다.
- PostgreSQL/OIDC runtime을 표준 command, image와 deployment example로 승격하는 작업은 별도 구현 milestone에서 검증한다.

## Alternatives Rejected

- 실제 consumer가 생길 때까지 P30을 미완료로 유지: repository-owned 구현과 배포 검증을 외부 adoption 상태에 종속시킨다.
- V0 runtime을 실제 predecessor로 간주해 모든 문서를 cutover 중심으로 유지: 존재하지 않는 운영·트래픽 전환을 제품 roadmap에 만든다.
- migration/recovery fixture를 삭제: optional V0 import와 authority recovery에 유효한 executable evidence까지 잃는다.
