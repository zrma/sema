# ADR 0007: Evaluation Calibration Baseline

- Status: Accepted

## Context

P6는 synthetic workload와 exhaustive oracle을 제공하지만 실제 rating model, production traffic distribution, target hardware와 numeric SLO는 아직 없다. 이 입력 없이 rating uncertainty formula나 elapsed-time threshold를 고정하면 임의의 제품 semantics와 환경 종속 gate가 된다.

## Decision

- 현재 `Player.Skill`은 rating point estimate로 해석한다.
- rating deviation/confidence와 uncertainty-aware objective는 실제 consumer의 rating contract가 생길 때 별도 ADR로 설계한다.
- P6 regression gate는 player coverage basis points, oldest unmatched wait, search nodes와 bounded oracle relation 같은 deterministic evidence를 사용한다.
- elapsed time과 allocation benchmark는 관찰 가능성을 유지하되 고정 pass/fail threshold를 두지 않는다.
- production cycle p95, capacity와 allocation ceiling은 target workload/hardware evidence가 생기는 P10에서 정한다.

## Consequences

- P6는 존재하지 않는 rating uncertainty를 임의 생성하거나 quality score에 섞지 않는다.
- planner optimization은 먼저 algorithmic coverage/cost/quality gate로 비교할 수 있다.
- machine performance regression은 반복 benchmark history가 생기기 전까지 review evidence이며 repository-wide hard failure가 아니다.

## Revisit Triggers

- consumer가 rating deviation, confidence interval 또는 placement uncertainty를 제공한다.
- target hardware와 queue distribution에 대한 repeatable benchmark environment가 생긴다.
- production cycle latency 또는 allocation이 deployment admission criterion이 된다.
