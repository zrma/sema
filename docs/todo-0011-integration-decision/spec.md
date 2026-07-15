# Next Integration Decision Spec

- Status: Decision Required

## Objective

same-process core와 offline policy evaluation 이후 첫 production-facing integration의 reliability, scale와 compatibility contract를 실제 consumer 요구로 선택한다.

## Required Input

1. 첫 실제 consumer가 같은 deployable process인지 별도 request/response service 또는 asynchronous worker인지.
2. producer가 restart 뒤 active demand와 policy를 전부 재제출할 수 있는지, durable recovery가 필요한지.
3. assignment acknowledgment가 synchronous call인지 retry 가능한 message delivery인지.
4. 첫 deployment가 single replica인지 multi-replica reservation authority가 필요한지.
5. cycle latency, queue wait, throughput과 audit retention의 최소 수치가 무엇인지.
6. remote module identity, public repository/API compatibility를 지금 고정해야 하는지.

## Recommended Default

실제 consumer와 수치 SLO가 없으면 현재 same-process engine, producer replay와 internal compatibility를 유지한다. transport, database, telemetry exporter와 public SDK는 추가하지 않고 simulation/benchmark evidence로 policy와 workload만 검증한다.

## Decision Consequences

- synchronous service는 request timeout, retry/idempotency mapping과 authentication contract가 필요하다.
- asynchronous worker는 durable inbox/outbox, ordering, duplicate delivery와 delayed acknowledgment가 필요하다.
- multi-replica는 external durable CAS authority와 assignment source of truth가 필요하다.
- public API는 module identity, schema version, compatibility와 release workflow를 함께 고정해야 한다.
- numeric SLO는 candidate index/partition, load/soak gate와 process extraction 판단의 기준이 된다.

## Safe Work Until Decision

- existing reference corpus에 game-specific fixture 추가.
- measured workload에서 재현되는 correctness/performance regression 수정.
- 현재 local gate와 documentation drift 유지보수.

위 입력 없이 새 protocol, database, distributed lock, public SDK 또는 timing threshold를 선택하지 않는다.
