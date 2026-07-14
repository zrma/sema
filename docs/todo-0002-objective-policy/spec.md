# P1 Objective Policy Spec

- Status: Active

## Objective

현재의 deterministic first-valid bounded enumeration을 explicit objective vector와 wait-based relaxation이 있는 candidate comparison으로 발전시킨다. hard constraint는 유지하면서 skill balance, role composition, wait time, cap 이내 latency의 선택 근거를 proposal evidence와 unmatched outcome으로 설명한다.

## Accepted Inputs

- canonical entity와 lifecycle: `docs/domain-model.md`, `docs/lifecycle.md`.
- product priority와 남은 parameters: `docs/todo-0001-foundation/decisions.md`.
- correctness와 workload envelope: `docs/reference-scenarios.md`, `docs/reference-workloads.md`.
- 실행 baseline: `internal/domain`, `internal/planner`, `internal/coordinator`.

## Deliverables

- versioned role requirement와 wait relaxation schedule을 표현하는 policy schema.
- hard constraint와 soft objective를 독립적으로 평가하는 package boundary.
- 후보를 deterministic objective vector로 비교하는 bounded planner.
- score breakdown과 unmatched reason contract.
- short-wait, long-wait, role, latency tie-break reference fixture.
- queue size별 benchmark와 search budget evidence.

## Acceptance

- 짧게 기다린 fixture는 더 낮은 team skill gap과 role penalty를 가진 후보를 선택한다.
- 오래 기다린 fixture는 policy가 허용한 범위에서 skill/role 조건을 완화하되 party, capacity, absolute latency cap을 위반하지 않는다.
- 동일한 quality 후보에서는 오래 기다린 ticket을 우선하고, 그다음 cap 이내 latency가 낮은 후보를 선택한다.
- proposal evidence만으로 후보 간 lexicographic 또는 weighted decision을 재현할 수 있다.
- match가 생기지 않으면 hard rejection, insufficient capacity, search budget 중 최소 하나의 stable unmatched reason을 제공한다.
- 같은 snapshot, policy, budget은 같은 ordered batch와 evidence를 만든다.
- 2:2부터 50:50 및 100인 party fixture에서 correctness gate를 유지하고 benchmark delta를 기록한다.

## Execution Order

1. objective vector와 role/relaxation policy schema를 fixture로 고정한다.
2. constraint와 scoring evaluator를 planner enumeration에서 분리한다.
3. bounded search가 best-known 후보를 비교하고 evidence를 보존하게 한다.
4. unmatched reason과 budget outcome을 추가한다.
5. workload matrix와 benchmark로 correctness, 결정성, 비용을 검증한다.
6. 수치가 실제 evidence를 얻으면 policy decision 또는 benchmark baseline으로 문서화한다.

## Out Of Scope

- rating system과 uncertainty model 자체의 구현.
- 모든 게임에 공통인 role taxonomy.
- global optimum 보장.
- distributed search와 durable state.
- public API compatibility와 remote publication.
