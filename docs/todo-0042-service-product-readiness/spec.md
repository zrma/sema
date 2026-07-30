# P31 Service Product Readiness Spec

- Status: In Progress — Standard Runtime Surface

## Objective

검증된 matcher와 authenticated PostgreSQL service를 저장소의 표준 runtime surface로 정리하고, 실제 게임 연동 없이도 반복 가능한 repository-owned evidence로 compatibility, high availability, observability와 recovery readiness를 입증한다.

## Fixed Inputs

- Sema는 기존 매치메이커나 실제 production traffic의 migration program이 아닌 standalone general-purpose matchmaking service다.
- PostgreSQL primary가 durable mutation authority이고 service replica는 stateless하다. Redis와 broker는 측정된 필요가 생기기 전 baseline에 추가하지 않는다.
- OIDC/JWT 검증은 provider-neutral resource-server contract를 유지하고 provider credential acquisition은 deployment 책임으로 둔다.
- matcher V0 conformance, tenant isolation, historical idempotency와 reservation/assignment authority를 runtime 정리 과정에서 약화하지 않는다.
- V0 journal/API는 development/reference 및 optional import compatibility surface이며 제품 기본 runtime의 장기 authority가 아니다.

## Implementation Sequence

- [x] standalone product scope와 reference deployment acceptance의 의미를 ADR로 고정한다.
- [x] PostgreSQL/OIDC service를 표준 command, container entrypoint와 deployment example로 승격한다.
- [x] V0 journal runtime을 명시적인 development/reference 또는 compatibility command로 분리하고 import path를 보존한다.
- [x] repository-owned reference client와 wire conformance suite로 전체 ticket-to-assignment lifecycle을 검증한다.
- [x] multi-replica contention, restart, dependency outage와 recovery matrix를 표준 runtime에 대해 실행한다.
- [x] workload 기반 admission, database pool/timeout과 service SLO를 sanitized repeatable report로 고정한다.
- [ ] metrics, tracing, alerting contract와 PostgreSQL backup/PITR recovery acceptance를 제품 runbook에 연결한다.
- [ ] compatibility/deprecation policy와 위 evidence가 충족된 뒤 stable admission을 별도 결정한다.

## Acceptance

- README, image, deployment example와 primary runbook이 같은 표준 runtime을 가리킨다.
- reference client가 인증 실패, tenant isolation, planning, reservation, assignment와 acknowledgment를 provider-neutral fixture에서 실행한다.
- 둘 이상의 stateless replica가 PostgreSQL authority 아래에서 demand double claim이나 terminal split-brain을 만들지 않는다.
- load/failure/recovery report가 raw credential, resource payload나 environment identity 없이 repeatable aggregate를 남긴다.
- V0 import를 사용하지 않는 신규 설치가 legacy journal이나 predecessor deployment를 요구하지 않는다.
- stable admission은 external game integration이 아니라 repository-owned compatibility와 operational evidence로 판정할 수 있다.

## Out Of Scope

- 게임 session 실행, player login, rating/MMR product와 실제 game traffic 운영.
- 특정 OIDC provider, Kubernetes distribution, database vendor 또는 private deployment inventory의 표준화.
- evidence 없는 cross-region multi-primary, broker, Redis, streaming과 public SDK 확장.
- 이번 milestone 시작 문서 변경과 동시에 runtime command를 무검증 rename하거나 기존 V0 import format을 삭제하는 작업.

## Decision Gate

표준 runtime 승격과 reference-client conformance는 repository-owned 작업으로 자율 진행한다. 기존 공개 command나 wire contract를 제거해야 하거나 stable compatibility 약속을 시작하는 시점에는 migration evidence와 별도 승인을 요구한다.
