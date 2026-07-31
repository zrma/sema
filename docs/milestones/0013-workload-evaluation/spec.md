# P6 Workload Evaluation Baseline Spec

- Status: Complete

## Objective

실제 production distribution이나 SLO를 섣불리 고정하지 않고 planner coverage, queue fairness와 bounded-search quality gap을 반복 측정할 deterministic P6 evaluation baseline을 만든다.

## Deliverables

- explicit seed와 weighted party/skill/role/latency/wait parameter를 가진 synthetic snapshot generator.
- ticket과 player를 분리한 coverage, oldest matched/unmatched wait와 proposal quality metric.
- 최대 two-team/12-ticket small-case exhaustive new-match oracle.
- planner 첫 proposal과 oracle quality vector의 relation report.
- synthetic multi-proposal queue와 intentional bounded-quality-gap lab workload.

## Acceptance

- 같은 model/seed는 같은 immutable scenario, 다른 seed는 다른 scenario를 만든다.
- generated party, skill, role, latency와 wait가 configured bounds를 벗어나지 않고 identity가 중복되지 않는다.
- coverage는 ticket이 아니라 player 기준 basis points를 함께 기록하고 oldest unmatched wait를 보존한다.
- 충분한 candidate budget의 small case는 oracle과 `equivalent`다.
- 의도적으로 candidate 1개로 제한한 fixture는 planner skill gap과 더 좋은 oracle gap을 함께 기록한다.
- oracle은 queue bound, new-match-only와 matching snapshot/batch contract를 거부 경로로 검증한다.
- focused test, race detector, executable smoke와 전체 repository gate가 통과한다.

## Out Of Scope

- production traffic sampling, PII와 실제 distribution calibration.
- rating uncertainty/confidence의 domain schema와 objective formula.
- global multi-proposal batch optimum 또는 backfill oracle.
- machine-specific elapsed-time threshold와 production SLO.

## Completion Evidence

- `internal/evaluation`이 deterministic generator, metrics와 exhaustive oracle을 소유한다.
- `sema-lab` `v0alpha2` report가 coverage basis points, oldest waits와 eligible oracle relation을 노출한다.
- seeded 5:5 queue와 bounded-gap diagnostic의 report outcome이 test에 고정된다.
- metric 의미, oracle 한계와 calibration boundary가 `docs/workload-evaluation.md`에 기록된다.

이 spec은 P6 evaluation mechanism을 닫는다. point-estimate rating boundary와 deterministic regression calibration은 ADR 0007 및 `docs/evaluation-baseline.md`가 이어서 고정한다.
