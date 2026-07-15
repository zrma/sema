# ADR 0003: Canonical Policy Fingerprint In Proposal Identity

- Status: Accepted
- Decision date: 2026-07-15

## Context

`MatchmakingPolicy.version`은 caller가 제공하는 label이어서 같은 version에 다른 rule content가 들어올 수 있다. 기존 proposal ID는 snapshot ID와 sequence만 사용하므로 다른 policy가 같은 placement를 만들 때 identity와 replay evidence가 policy 변경을 드러내지 못한다.

## Decision

- validated policy의 모든 field를 SHA-256 canonical fingerprint에 포함한다.
- role requirement는 unique role name으로 정렬해 입력 slice 순서 차이를 제거한다.
- relaxation step은 wait schedule의 의미 있는 순서를 그대로 보존한다.
- proposal은 `policyVersion`과 full `policyFingerprint`를 함께 기록한다.
- proposal ID는 snapshot ID, sequence, policy version/fingerprint, proposal kind, canonical team placement와 backfill target을 반영한 content digest를 포함한다.

## Consequences

- 같은 policy content와 placement는 반복 실행에서 같은 identity를 만든다.
- 같은 version이라도 rule content가 다르면 fingerprint와 proposal ID가 달라진다.
- coordinator reservation idempotency는 다른 policy content의 proposal을 exact payload conflict로 구분한다.
- fingerprint는 content identity이며 authorization이나 cryptographic signature가 아니다.

## Follow-up

process lifetime에서 같은 version의 다른 fingerprint를 거부하는 contract는 별도 policy catalog milestone에서 구현한다. public serialization, registry distribution과 migration policy는 external consumer 요구가 생길 때 결정한다.
