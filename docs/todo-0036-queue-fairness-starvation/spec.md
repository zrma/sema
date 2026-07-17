# P25 Queue Fairness And Starvation Spec

- Status: Completed

## Objective

quality가 좋은 새 demand가 지속적으로 들어와도 policy가 wait-priority로 선언한 오래된 feasible demand가 영구히 밀리지 않게 한다. wait relaxation, oldest-prefix discovery와 global batch selector가 같은 age/service contract를 사용하고, bounded search 밖의 demand를 보장한 것처럼 표시하지 않는다.

## Service Ordering

각 match ticket과 backfill ticket은 자신의 `enqueuedAt`으로 active relaxation step을 계산한다. `PrioritizeWait`가 true이고 admissible candidate에 포함된 demand를 priority-eligible로 본다.

batch selection은 다음 순서를 사용한다.

1. selected backfill 수.
2. selected batch가 service하는 가장 오래된 priority demand wait.
3. oldest wait가 같을 때 selected priority demand 수.
4. small default boundary의 coverage/Pareto ordering 또는 일반 bounded path의 rank utility.
5. canonical key.

oldest service가 utility보다 앞서므로 같은 backfill tier의 fresh quality가 aged feasible demand를 무한히 우회할 수 없다. 한 cycle에서 모든 priority demand를 채운다는 뜻은 아니며 selected demand가 queue에서 제거된 다음 cycle에 다음 oldest가 승격된다.

## Evidence And Failure Boundary

`BatchScoreEvidence`는 bounded candidate graph에서 다음 값을 반환한다.

- `WaitPriorityEligibleDemands`: priority demand를 포함한 admissible candidate의 distinct demand 수.
- `WaitPrioritySelectedDemands`: selected batch가 service하는 priority demand 수.
- `OldestWaitPriorityMillis`: eligible priority demand의 최대 wait.
- `OldestSelectedPriorityMillis`: selected priority demand의 최대 wait.

hard-rejected demand는 eligible에 포함되지 않는다. candidate에는 있었지만 conflict, proposal limit 또는 batch objective로 선택되지 않은 demand는 eligible-selected 차이로 관찰한다. candidate window, generation 또는 selection이 truncated되면 `BudgetExhausted`가 true이며 service invariant는 보장되지 않는다. insufficient capacity와 quality threshold는 기존 unmatched reason을 유지한다.

## Acceptance

- 30초 전에는 gap 0 fresh pair가 선택되고 30초 priority boundary의 첫 cycle에는 오래된 gap 1000 pair가 선택된다.
- 매 10초 fresh pair가 계속 유입되어도 old pair service wait는 fixture policy boundary 30000ms로 bounded된다.
- direct selector는 proposal 수가 같은 경우 높은 fresh rank-sum보다 oldest priority demand를 포함한 batch를 선택한다.
- match ticket과 backfill ticket의 own age가 priority eligibility와 evidence에 반영된다.
- no-priority 50v50와 100K fast path allocation 수준을 유지한다.
- public Go marker/migration, service DTO additive evidence, durable replay와 full/race/publication gate를 통과한다.

## Truth Boundary

- 30초는 deterministic fixture parameter이며 production maximum-wait SLA가 아니다.
- admissible candidate가 bounded graph에 생성되었다는 전제의 service guarantee다. explicit approximation 뒤의 feasible placement를 증명하지 않는다.
- backfill-first priority는 유지한다. 서로 다른 priority class 사이의 product policy는 P26 이후 calibration 대상이다.
- ticket count 기준 service이며 party player 수 기반 weighted fairness는 별도 consumer decision이다.

## Completion Evidence

- sustained-arrival planner regression이 cycle 0/10/20초의 fresh selection과 30초 old-pair selection을 고정한다.
- direct selector regression이 utility 200 fresh batch 대신 utility 61 aged batch를 선택한다.
- priority evidence가 eligible/selected 2개와 oldest eligible/selected 30000ms를 기록한다.
- no-priority reference benchmark는 priority slice allocation을 만들지 않는다.
