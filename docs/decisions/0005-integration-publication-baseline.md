# ADR 0005: Initial Integration And Publication Baseline

- Status: Superseded by ADR 0009 and ADR 0011

## Context

domain core, in-process runtime validation과 offline policy simulation은 완료되었지만 실제 production consumer와 수치 SLO는 아직 없다. 동시에 source repository를 공개하려면 remote module identity, license와 compatibility boundary를 모호하지 않게 고정해야 한다.

## Decision

- 첫 integration은 같은 Go process에서 `internal/engine`을 직접 호출한다.
- restart recovery는 producer가 active ticket, backfill과 policy를 재제출하는 방식이다.
- assignment acknowledgment는 synchronous call이고 첫 deployment는 single replica다.
- repository와 Go module identity는 `github.com/zrma/sema`다.
- source는 Apache License 2.0으로 공개한다.
- 모든 Go package는 `internal/`에 유지하며 public SDK, semantic-version compatibility와 migration guarantee는 아직 제공하지 않는다.
- production SLO는 실제 consumer와 deployment evidence가 생길 때 정한다.

## Consequences

- 공개된 source를 검토하고 실행할 수 있지만 외부 Go module은 현재 package를 import할 수 없다.
- transport, durable storage와 distributed coordination을 요구사항 없이 고정하지 않는다.
- public API를 추가할 때는 package boundary, schema, compatibility와 release workflow를 함께 설계한다.
- public push는 repository publication gate와 권한 있는 machine-local inventory gate를 모두 통과해야 한다.
