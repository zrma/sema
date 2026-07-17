# ADR 0011: Versioned Single-Writer HTTP Service Baseline

- Status: Accepted

## Context

P8 public alpha는 side-effect-free composition만 제공하고 P9 durable runtime은 internal Go caller만 사용할 수 있다. 별도 process producer/consumer가 ticket ingestion과 assignment lifecycle을 실행하려면 wire contract가 필요하다. 반면 authentication, multi-replica authority와 remote database 요구는 아직 없다.

## Decision

- `cmd/sema-server`가 stdlib HTTP server와 explicit `internal/api/v0alpha1` JSON DTO를 제공한다.
- ticket, backfill, plan, reserve, confirm, assignment poll과 terminal acknowledgment를 versioned endpoint로 노출한다.
- server clock이 wait/TTL/lifecycle time authority다.
- reserve는 client proposal body를 받지 않고 durable plan audit에서 찾은 `proposal_id`만 허용한다.
- accepted call은 durable journal sync 뒤 response를 반환한다.
- delivery는 synchronous response, caller-chosen idempotency ID와 assignment polling이다.
- planner, coordinator, journal writer와 HTTP adapter는 하나의 process/deployable에 둔다.
- default listener는 loopback이고 non-loopback unauthenticated bind는 explicit unsafe flag를 요구한다.

## Consequences

- Go 외 consumer도 restart-safe lifecycle을 호출할 수 있다.
- wire DTO와 domain evolution을 분리하고 unknown field를 거부한다.
- forged proposal로 planner constraint를 우회할 수 없다.
- 한 process가 planning과 journal sync를 직렬화하므로 horizontal write scaling은 없다.
- authentication, TLS, rate limit, push delivery와 outbox는 후속 production integration이 필요하다.

## Process Separation Review

별도 consumer process 경계는 HTTP로 확인되었지만 planner와 coordinator를 분리할 evidence는 없다. reservation authority와 journal ordering을 한 writer에 유지하는 편이 failure semantics가 단순하다. horizontal coordination은 measured throughput 또는 availability requirement가 single writer를 초과할 때 다시 설계한다.

## Revisit Triggers

- non-loopback production deployment와 identity/authorization requirement.
- polling latency나 delivery loss가 durable outbox/subscriber를 요구한다.
- measured plan or journal throughput이 one-writer SLO를 넘는다.
- rolling upgrade, multi-replica HA 또는 external database가 필요하다.
