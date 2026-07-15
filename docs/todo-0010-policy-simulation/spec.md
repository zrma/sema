# P4 Policy Simulation Spec

- Status: Complete

## Objective

versioned policy를 fixed snapshot corpus에 side effect 없이 실행하고 결과와 decision-audit metric을 비교하는 offline simulation boundary를 만든다. production coordinator state를 변경하지 않고 policy 후보의 coverage와 quality evidence를 평가한다.

## Deliverables

- registered policy와 immutable snapshot을 입력받는 simulation runner.
- policy별 proposal, matched/unmatched reason, search budget과 score evidence summary.
- 같은 corpus/policy 반복 실행과 policy order 독립성 fixture.
- reference team, battle royale, backfill과 no-match corpus.

## Acceptance

- simulation은 planner만 호출하고 coordinator/reservation/assignment state를 만들지 않는다.
- 같은 policy/corpus는 ordered result와 summary가 같다.
- 여러 policy 입력 순서를 바꿔도 version/fingerprint별 결과가 같다.
- invalid 또는 conflicting policy는 typed failure로 simulation 전에 거부된다.
- focused simulation test, race detector와 전체 repository gate가 통과한다.

## Out Of Scope

- production traffic sampling과 PII ingestion.
- policy winner 자동 activation과 rollout.
- file/database schema, remote job runner와 UI.
- game-specific quality threshold 결정.

## Completion Evidence

- `internal/simulation.Run`이 policy 후보를 catalog에서 선검증하고 immutable scenario를 planner에만 전달한다.
- report가 policy version/fingerprint와 scenario ID 순으로 canonicalize된다.
- result가 full `ProposalBatch`, matched/unmatched reason, search budget과 score summary를 함께 제공한다.
- policy/scenario order independence, conflict rejection과 team/battle-royale/backfill/no-match fixture가 통과한다.
- focused simulation test, race detector와 전체 repository gate가 통과한다.

입출력과 summary contract는 `docs/policy-simulation.md`가 소유한다.
