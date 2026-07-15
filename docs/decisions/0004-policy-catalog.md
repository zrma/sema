# ADR 0004: Explicit Process-Local Policy Catalog

- Status: Accepted
- Decision date: 2026-07-15

## Context

canonical fingerprint는 같은 version의 다른 content를 구분하지만, runtime이 둘을 모두 planning에 사용할 수 있으면 version label 자체는 안정적인 contract가 아니다. same-process adapter에는 외부 registry나 durable activation 요구가 아직 없다.

## Decision

- engine은 in-memory policy catalog를 소유한다.
- consumer는 planning 전에 full `MatchmakingPolicy`를 명시적으로 등록한다.
- first registration은 version과 canonical fingerprint를 defensive policy copy에 묶는다.
- same version/same fingerprint registration은 idempotent하다.
- same version/different fingerprint registration은 `PolicyConflict`이며 기존 entry를 변경하지 않는다.
- `Plan`과 `Snapshot`은 full policy가 아니라 registered policy version을 받고 catalog copy를 사용한다.

## Consequences

- planning 호출은 catalog state를 암묵적으로 변경하지 않는다.
- caller가 registration input이나 read result를 수정해도 stored policy는 바뀌지 않는다.
- concurrent first registration은 하나의 content만 선택한다.
- process restart 뒤 catalog는 비어 있으며 consumer가 policy를 다시 등록해야 한다.

## Deferred

durable registry, activation schedule, authorization, remote distribution과 schema migration은 실제 deployment/consumer 요구가 생길 때 함께 결정한다.
