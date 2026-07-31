# Initial Integration And Publication Spec

- Status: Complete

## Objective

same-process core와 offline policy evaluation 이후 첫 integration, durability, deployment와 publication baseline을 명시적으로 고정한다.

## Decision

1. 첫 consumer는 같은 Go deployable process에서 `internal/engine`을 직접 호출한다.
2. process restart 뒤 producer가 active ticket, backfill과 policy를 전부 재제출한다.
3. assignment acknowledgment는 synchronous call로 처리한다.
4. 첫 deployment는 single replica로 운영한다.
5. cycle latency, queue wait, throughput과 audit retention의 production SLO는 실제 deployment input이 생길 때 정한다.
6. repository와 Go module identity는 `github.com/zrma/sema`로 공개하되 모든 package를 `internal/`에 유지하고 public API compatibility는 약속하지 않는다.
7. source license는 Apache License 2.0이다.

## Consequences

- transport, database, telemetry exporter, distributed lock과 public SDK를 미리 추가하지 않는다.
- restart recovery는 durable state가 아니라 producer replay의 정확성과 idempotency에 의존한다.
- multi-replica 전환에는 external durable CAS authority와 assignment source of truth가 필요하다.
- public API를 열 때는 package boundary, schema version, compatibility와 release workflow를 함께 결정한다.
- numeric SLO가 생기면 candidate index/partition, load/soak gate와 process extraction 여부를 재평가한다.

## Evidence

- `internal/engine` lifecycle fixture가 direct call, replay와 synchronous acknowledgment를 검증한다.
- reference benchmark가 현재 workload의 비교 가능한 local measurement를 제공한다.
- `scripts/check.sh`가 module, race, benchmark와 publication boundary contract를 검증한다.

## Revisit Triggers

- consumer가 별도 process 또는 asynchronous worker를 요구한다.
- producer가 restart 뒤 active demand를 완전하게 replay할 수 없다.
- multi-replica coordination이나 durable audit retention이 필요하다.
- 외부 프로젝트가 안정적인 Go API 또는 versioned schema를 요구한다.
- production SLO가 현재 in-process baseline으로 충족되지 않는다.
