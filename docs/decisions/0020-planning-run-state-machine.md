# ADR 0020: Repository-Versioned Planning Run State Machine

- Status: Accepted

## Context

matcher search는 queue 크기와 policy budget에 따라 database transaction보다 오래 실행될 수 있다. search 동안 repository transaction이나 scope lock을 유지하면 unrelated ingress를 막고 replica contention을 correctness 문제로 바꾼다. 반대로 현재 queue를 매 retry마다 다시 읽으면 response loss나 process interruption 뒤 같은 planning command가 다른 proposal을 만들 수 있다.

## Decision

planning run은 두 개의 atomic repository commit 사이에서 side-effect-free matcher를 실행한다.

1. authenticated command는 policy와 active MatchTicket/BackfillTicket을 한 repository version에서 읽는다.
2. immutable `planning_snapshot`과 `planning` 상태의 `planning_run`을 operation receipt와 함께 짧은 commit으로 저장한다.
3. transaction을 모두 닫은 뒤 저장된 snapshot으로 matcher를 실행한다.
4. completed run, proposal와 unmatched resource 전체를 deterministic internal completion operation으로 한 commit에 기록한다.
5. capture 뒤 중단되면 같은 client operation ID/digest retry가 저장된 snapshot을 다시 읽어 3단계부터 재개한다.

completion operation ID는 client가 사용할 수 없는 internal namespace에 있고 digest는 server completion time을 제외한 deterministic result 전체에 묶인다. 두 replica가 같은 captured snapshot을 동시에 계산하면 한 completion만 commit되고 다른 replica는 authoritative completed run을 읽는다. matcher가 다른 결과를 만들면 idempotency conflict가 nondeterminism을 드러낸다.

proposal와 unmatched page cursor는 계속 변하는 tenant repository version이 아니라 immutable completed run의 storage version에 묶인다. 따라서 unrelated queue ingress 뒤에도 같은 planning result page를 계속 읽을 수 있다.

## Consequences

- matcher latency는 queue ingress transaction을 열어 두거나 차단하지 않는다.
- response loss와 planner interruption은 현재 queue를 다시 capture하지 않고 original snapshot으로 재개된다.
- proposal/unmatched 결과와 run count가 부분적으로 보이는 상태는 없다.
- planning snapshot은 현재 full input payload를 보존한다. kind-specific snapshot query, chunking과 large-result write optimization은 measured production workload가 요구할 때 추가한다.
- pending run 재개에는 original client operation identity가 필요하다. background worker ownership과 orphan recovery policy는 remote runtime orchestration을 정할 때 추가한다.

## Revisit Triggers

- representative snapshot/result 크기가 PostgreSQL transaction 또는 resource payload budget을 넘는다.
- asynchronous worker, cancellation, deadline 또는 orphan takeover가 consumer requirement가 된다.
- result delivery가 HTTP polling보다 강한 stream/outbox ordering을 요구한다.
