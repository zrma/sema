# P1 Objective Policy Contract

## Boundary

P1은 rating system 자체가 아니라 이미 계산된 `Player.skill`, `Player.role`, `Player.latencyMillis`, `MatchTicket.enqueuedAt`을 조합하는 정책을 정의한다. 수치는 versioned `MatchmakingPolicy`가 소유하고 planner 코드에 숨기지 않는다.

## Role Requirement

각 `RoleRequirement`는 다음 값을 가진다.

- `role`: player의 exact role label.
- `minPerTeam`: 새 match의 각 team에 필요한 최소 인원.
- `hard`: `true`이면 모든 relaxation 단계에서 반드시 충족하고, `false`이면 부족한 인원 수를 role penalty로 계산한다.

role taxonomy와 한 player의 multi-role 표현은 P1 범위 밖이다. P1 당시 backfill은 기존 roster의 role 정보가 없어 role/skill quality threshold를 적용하지 않고 새로 들어가는 party의 wait와 latency만 비교했다. P26부터 optional roster-versioned aggregate가 있으면 resulting roster의 role/skill/latency evidence를 같은 objective vocabulary로 평가하고, context가 없을 때만 이 legacy behavior를 유지한다.

## Relaxation Step

각 `RelaxationStep`은 다음 값을 가진다.

- `afterWait`: proposal에서 가장 오래 기다린 ticket의 wait가 이 값 이상일 때 활성화된다.
- `maxTeamSkillGap`: team별 평균 skill의 최대값과 최소값 차이에 대한 허용 상한.
- `maxRolePenalty`: 모든 team의 soft role 부족 인원 합계에 대한 허용 상한.
- `prioritizeWait`: 후보 비교에서 wait를 quality보다 먼저 둘지 결정한다.

첫 단계는 `afterWait=0`이어야 하고 이후 threshold는 strictly increasing이다. skill/role 허용 상한은 뒤 단계로 갈수록 같거나 넓어져야 한다. `prioritizeWait`는 `false`에서 `true`로만 전이할 수 있다.

relaxation step이 생략된 policy는 기존 P0 fixture 호환을 위해 skill/role 상한이 없는 단일 quality-first 단계로 해석한다. 실제 policy adapter는 명시적 step과 version을 제출해야 한다.

## Objective Evidence

proposal은 다음 값을 보존한다.

- `relaxationLevel`, `waitPriority`.
- `rolePenalty`, `teamSkillGap`.
- `oldestWaitMillis`, `totalWaitMillis`.
- `maxLatencyMillis`.
- `candidatesEvaluated`, `searchNodes`, `searchTruncated`.

같은 policy에서 이 evidence와 canonical ticket ordering만으로 선택 결과를 재현할 수 있어야 한다.

## Candidate Ordering

quality-first 단계는 다음 lexicographic order를 사용한다.

1. lower `rolePenalty`.
2. lower `teamSkillGap`.
3. higher `oldestWaitMillis`.
4. higher `totalWaitMillis`.
5. lower `maxLatencyMillis`.
6. canonical team/ticket ordering.

wait-first 단계는 다음 order를 사용한다.

1. higher `oldestWaitMillis`.
2. higher `totalWaitMillis`.
3. lower `rolePenalty`.
4. lower `teamSkillGap`.
5. lower `maxLatencyMillis`.
6. canonical team/ticket ordering.

두 후보의 active step이 다르고 하나라도 wait-first이면 wait-first order를 사용한다. hard role 또는 현재 step의 skill/role 상한을 넘는 후보는 비교 전에 제외한다.

## Unmatched Reasons

- `hard_constraint`: absolute latency, party size, hard role처럼 완화할 수 없는 조건 위반.
- `insufficient_capacity`: 현재 party shape로 정확한 team/session capacity를 채울 수 없음.
- `quality_threshold`: exact placement는 있지만 active relaxation step의 skill/soft-role 상한을 넘음.
- `search_budget`: node budget 안에서 결론을 내리지 못함.
- `proposal_limit`: policy의 batch proposal 상한에 도달함.

P1 reason은 stable category이며 하나의 대표 원인을 제공한다. 상세한 per-rule trace와 복수 원인 집계는 observability milestone에서 확장한다.

## Reference Policy Values

P1 fixture는 동작을 구분하기 위해 다음 값을 사용한다. 제품 기본값이나 SLO가 아니다.

- short step: `afterWait=0`, `maxTeamSkillGap=50`, `maxRolePenalty=0`, quality-first.
- relaxed step: `afterWait=30s`, `maxTeamSkillGap=200`, `maxRolePenalty=2`, wait-first.
- absolute latency cap: `200ms`.
- candidate cap: proposal당 `64`.
