# Agent Harness

## Interface

- Structure ID: `agent-harness-v1`.
- Baseline ID: `openai-gpt-5.6-2026-07-11`.
- Convergence stage: `canonical`.
- Target stage: `canonical`.
- Canonical check: `scripts/check-agent-harness-interface.sh`.
- Publication class: `public`.
- Publication boundary check: `scripts/check-publication-boundary.py`.

`AGENTS.md`가 공통 GPT-5.6 계약을 소유하고, 이 문서는 Sema의 matchmaking architecture와 현재 foundation 작업으로 가는 canonical 진입점이다.

Sema는 public source repository다. 원격 visibility와 repository 선언은 항상 일치해야 하며 public push 전에는 repository gate와 권한 있는 machine-local private-inventory gate를 모두 통과한다. `alpha`는 experimental public composition API이고 그 외 Go package는 `internal/`에 머문다. stable API compatibility는 아직 약속하지 않는다.

Tracked artifact contract: raw tool output와 정확한 로컬 환경 evidence는 local-only로 취급한다. 공개 가능한 기록에는 repository-owned 결정, 필요한 명령 이름, redacted 검증 판정만 남기고 경로·호스트·주소·클러스터 값은 placeholder로 바꾼다.

## Project Objective

대기 중인 플레이어·파티와 기존 세션의 빈자리를 대상으로 hard constraint를 만족하는 다양한 후보 조합을 탐색하고, 품질과 운영 비용을 함께 고려한 multi-match `ProposalBatch`와 확정 assignment를 생성한다.

## Source Of Truth

- 제품 목적과 용어: `README.md`; 시스템 경계와 invariant: `docs/architecture.md`.
- 현재 baseline과 리스크: `docs/status.md`; milestone 순서와 non-goals: `docs/roadmap.md`.
- 무컨텍스트 시작점: `docs/HANDOFF.md`; 현재 작업 계약: 활성 `docs/todo-*/spec.md`.
- repository entrypoint와 검증 선언: `docs/REPO_MANIFEST.yaml`.

## Autonomy And Permissions

- 목표, acceptance, 검증 경로가 명확한 로컬·가역 작업은 추가 승인 없이 조사, 구현, 검증, 문서화, local `jj` change 정리까지 진행한다.
- 외부 write, secret, 비용, 파괴적 작업, 공개 범위 변경, published history rewrite, 승인되지 않은 push는 에스컬레이션한다.
- domain semantics, consistency guarantee, implementation stack처럼 여러 합리적 선택이 장기 계약을 바꾸면 evidence와 tradeoff를 정리한 뒤 최소 판단만 요청한다.

## Execution Loop

1. `jj status`, handoff/status/roadmap, 활성 todo를 확인한다.
2. discovery, constraint, scoring, proposal batch, reservation, assignment 중 이번 논리 경계를 고정한다.
3. 입력 fixture, invariant, observable outcome, acceptance evidence를 먼저 정의한다.
4. side effect 없는 가장 작은 vertical slice를 구현하고 focused validation을 즉시 실행한다.
5. persistence나 coordination이 필요하면 idempotency와 failure transition을 테스트로 고정한다.
6. `scripts/check.sh`까지 넓혀 실패를 같은 루프에서 닫고 durable decision만 문서에 반영한다.
7. 하나의 목적을 가진 `jj` change로 닫고 외부 write 전에는 승인을 받는다.

## Verification And Evidence

- 전체 local gate: `scripts/check.sh`.
- Harness interface: `scripts/check-agent-harness-interface.sh`.
- Publication boundary: `scripts/check-publication-boundary.py`; 공개 출고 전에는 권한 있는 local private-inventory guard도 실행한다.
- architecture 단계에서는 domain fixture, state transition, deterministic replay, failure behavior가 acceptance evidence다.
- runtime이 추가되면 해당 언어의 format, lint, test, build와 최소 end-to-end matching scenario를 전체 gate에 편입한다.
- 최종 evidence에는 local result, 남은 리스크, local/remote bookmark, CI 상태를 서로 구분해 포함한다.

## Escalation

제품 semantics, fairness와 latency의 수치, accepted implementation baseline 변경, consistency guarantee 확장, credential·비용, 파괴적 변경, visibility 변경, published history rewrite, 승인되지 않은 push가 필요할 때만 사용자에게 최소 판단을 요청한다.

## VCS And Publish

- 로컬 VCS는 `jj`를 사용하고 change description은 `<type>: <summary>`와 Codex attribution trailer 규칙을 따른다.
- 변경은 independently explainable하고 검증 가능한 milestone 단위로 유지한다.
- push, tag, release, visibility 변경은 별도 외부-write 경계이며 명시적 권한 없이 실행하지 않는다.
- 공개 전에는 repository publication gate와 권한 있는 machine-local inventory gate를 모두 통과한다.

## Harness Evaluation And Improvement

대표 new-match와 backfill fixture에서 완료성, batch 전체의 matched coverage와 proposal 품질, constraint 위반, 결정성, 설명 가능성, 탐색 latency, resource cost를 평가한다. 반복 실패는 domain test, benchmark, validation script, concise operating rule 중 가장 가까운 계층에 기계화한다.

## Convergence

- `bridge`: 이 문서가 공통 인터페이스를 제공하고 기존 상세 문서를 연결한다.
- `normalized`: autonomy, execution, verification, escalation, VCS 정책을 동일 섹션 계약으로 이동한다.
- `canonical`: 프로젝트 목적과 domain invariant는 local content로 유지하고 공통 baseline, 제목 순서, 검사 골격을 잠근다.
- 단계 전환은 현재 저장소의 Structure ID, 섹션 순서, canonical check 결과로 검증하며 다른 저장소의 이름·개수·로컬 경로·공개 여부를 전제하지 않는다.

## Project Overlay

- `MatchTicket`은 플레이어·파티가 새 세션을 찾는 수요이고 `BackfillTicket`은 기존 세션이 빈자리를 채우는 수요다.
- planner는 immutable snapshot을 받아 서로 ticket이 겹치지 않는 `ProposalBatch`를 만들며 외부 상태를 직접 변경하지 않는다.
- coordinator는 batch의 proposal을 reservation과 assignment로 전이시키며 모든 mutation을 idempotent하게 만든다.
- 구현 baseline은 ADR 0001의 Go 단일 프로세스·인메모리 구조이며 package boundary를 process boundary보다 먼저 검증한다.
- hard constraint 위반은 점수로 보상하지 않고 후보에서 제외한다. soft objective와 tradeoff는 evidence로 노출한다.
- 저장소와 transport 구현은 domain core를 import할 수 있지만 domain core는 외부 adapter를 알지 못한다.
- public `alpha.Compose`는 internal type을 alias하지 않고 immutable input/output을 명시적으로 변환하며 coordinator side effect를 노출하지 않는다.
- tag/release는 workflow 존재 여부와 무관하게 별도 외부-write 승인과 publication/private-inventory gate를 요구한다.
- durable runtime mutation은 journal sync 뒤에만 성공이며 replay schema, TTL과 single-writer authority를 우회하지 않는다.
- service reserve는 durable plan의 proposal ID만 받아 planner를 우회한 placement를 authority로 만들지 않는다.

## Related Documents

- Navigation: `docs/HANDOFF.md`.
- Current state and direction: `docs/status.md`, `docs/roadmap.md`.
- Architecture: `docs/architecture.md`.
- Workloads and decisions: `docs/reference-workloads.md`, `docs/decisions/0001-implementation-baseline.md`.
- Completed foundation: `docs/todo-0001-foundation/spec.md`, `docs/todo-0001-foundation/decisions.md`.
- Completed objective policy: `docs/todo-0002-objective-policy/spec.md`.
- Completed assignment lifecycle: `docs/todo-0003-assignment-lifecycle/spec.md`.
- Completed application runtime: `docs/todo-0004-application-runtime/spec.md`.
- Completed runtime adapter decision: `docs/todo-0005-runtime-adapter/spec.md`, `docs/decisions/0002-runtime-adapter-baseline.md`.
- Completed runtime validation: `docs/todo-0006-runtime-validation/spec.md`, `docs/runtime-validation.md`.
- Demand index: `docs/todo-0007-demand-index/spec.md`.
- Policy identity: `docs/todo-0008-policy-identity/spec.md`.
- Policy catalog: `docs/todo-0009-policy-catalog/spec.md`.
- Policy simulation: `docs/todo-0010-policy-simulation/spec.md`.
- Completed integration and publication baseline: `docs/todo-0011-integration-decision/spec.md`, `docs/decisions/0005-integration-publication-baseline.md`.
- Executable evaluation lab: `docs/todo-0012-sema-lab/spec.md`, `docs/sema-lab.md`.
- Workload evaluation baseline: `docs/todo-0013-workload-evaluation/spec.md`, `docs/workload-evaluation.md`.
- Evaluation regression and calibration: `docs/evaluation-baseline.md`, `docs/decisions/0007-evaluation-calibration-baseline.md`.
- Candidate discovery and scale: `docs/todo-0014-candidate-discovery/spec.md`, `docs/candidate-discovery.md`, `docs/decisions/0008-candidate-window-baseline.md`.
- Public alpha integration: `docs/todo-0015-public-integration/spec.md`, `docs/public-api.md`, `docs/api-compatibility.md`, `docs/releasing.md`, `docs/decisions/0009-alpha-integration-release-baseline.md`.
- Durable runtime: `docs/todo-0016-durable-runtime/spec.md`, `docs/durable-runtime.md`, `docs/decisions/0010-durable-journal-baseline.md`.
- Versioned service: `docs/todo-0017-http-service/spec.md`, `docs/service-api.md`, `docs/decisions/0011-http-service-baseline.md`.
- Long-term engine-first sequence: `docs/decisions/0006-product-development-sequence.md`.
- Declared checks: `docs/REPO_MANIFEST.yaml`.
