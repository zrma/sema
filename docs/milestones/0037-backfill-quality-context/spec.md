# P26 Backfill Quality Context Spec

- Status: Completed

## Objective

vacancy capacity만 채우던 backfill을 existing roster와 incoming placement의 resulting quality까지 평가하도록 확장한다. context와 quality evidence는 backfill ticket revision 및 `rosterVersion`에 묶고, new match와 같은 skill/role/latency vocabulary를 사용하면서 backfill-first product priority는 유지한다.

## Aggregate Contract

`BackfillTicket.ExistingTeams`는 optional team-order aggregate다.

- `PlayerCount`: 현재 team player 수.
- `SkillTotal`: 현재 team player skill 합.
- `RoleCounts`: role별 player 수. 빈 role player는 합계에 포함하지 않아도 된다.
- `MaxLatencyMillis`: 현재 roster에서 관측한 maximum latency.

context가 있으면 team 수는 `OpenSlotsByTeam`과 같고 `PlayerCount + open slots == policy.TeamSize`여야 한다. negative 값, duplicate role과 player count보다 큰 role 합은 invalid다. context가 없으면 legacy vacancy-only behavior를 사용한다.

## Resulting Quality

planner는 existing aggregate에 incoming players를 team별로 더한 뒤 다음 evidence를 계산한다.

- team average skill의 maximum gap.
- policy role requirement의 resulting hard violation 또는 soft deficit.
- existing/incoming player의 maximum latency.
- incoming ticket와 backfill demand wait/priority evidence.

backfill target의 ticket revision과 `rosterVersion`이 quality context freshness를 대표한다. context를 바꾸는 producer는 revision을 전진시키고 실제 roster 변경이면 roster version도 전진시킨다. reserve는 proposal의 두 freshness 값과 active demand를 CAS한다.

## Acceptance

- asymmetric skill/role roster에 high-dps/low-healer를 보정 배치해 resulting gap 0과 role penalty 0을 만든다.
- existing max latency 60이 proposal evidence에 보존된다.
- planner와 exhaustive frontier가 roster-aware quality point에서 equivalent다.
- public Go and service DTO가 aggregate를 defensive conversion하고 durable journal이 restart replay한다.
- higher ticket/roster revision의 context가 ingest되면 이전 proposal reserve는 stale다.
- empty aggregate의 기존 backfill fixture와 P24 frontier corpus가 유지된다.
- public alpha marker/migration과 full/race/publication gate를 통과한다.

## Truth Boundary

- aggregate는 point-estimate input이며 rating uncertainty, multi-role semantics나 per-region latency matrix를 정의하지 않는다.
- `MaxLatencyMillis`는 existing roster 관찰 evidence다. incoming player의 absolute cap은 계속 hard constraint다.
- 같은 revision/version에서 producer가 다른 aggregate를 보내는 것은 contract violation이며 cross-producer reconciliation은 service productization 범위다.
- full roster player identity를 matcher input이나 public evidence에 복제하지 않는다.

## Completion Evidence

- planner/public alpha fixture는 `rosterVersion=7`, gap 0, role penalty 0, max latency 60을 반환한다.
- exhaustive frontier가 같은 planner batch를 `frontier_equivalent`로 분류한다.
- coordinator stale-backfill fixture가 context와 revision/version 전진 뒤 이전 proposal reserve를 거부한다.
- invalid capacity/context validation과 HTTP DTO ingestion regression이 통과한다.
