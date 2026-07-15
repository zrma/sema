# P4 Policy Identity Spec

- Status: Complete

## Objective

caller가 제공한 policy version을 실제 policy content와 결합해 replay와 reservation evidence가 같은 이름의 다른 rule set을 구분하도록 한다. snapshot sequence만으로 만들어지는 proposal identity가 policy 변경을 숨기지 않게 한다.

## Deliverables

- 모든 `MatchmakingPolicy` field를 포함하는 deterministic canonical fingerprint.
- proposal evidence에 policy version과 fingerprint를 함께 기록한다.
- proposal identity가 snapshot identity, policy fingerprint와 canonical placement를 반영한다.
- 같은 content의 clone/order contract와 같은 version의 다른 content fixture를 추가한다.

## Acceptance

- 같은 policy content는 반복 실행과 defensive copy 뒤에도 같은 fingerprint를 만든다.
- 같은 version이라도 rule content가 다르면 fingerprint와 proposal identity가 다르다.
- 같은 snapshot, policy와 placement는 같은 proposal identity를 만든다.
- reserve retry는 exact proposal identity/content에 대해 idempotent하고 다른 policy proposal은 충돌한다.
- focused domain/planner/coordinator test, race detector와 전체 repository gate가 통과한다.

## Out Of Scope

- policy registry, distribution과 authorization.
- public serialization schema와 compatibility guarantee.
- cryptographic signature와 trust chain.
- game-specific skill metric과 role catalog.

## Completion Evidence

- `domain.FingerprintPolicy`가 validated policy 전체를 canonical SHA-256 identity로 만든다.
- role requirement order는 canonicalize하고 relaxation schedule order는 보존한다.
- proposal이 policy version/fingerprint를 기록하며 ID가 snapshot, policy, kind, placement와 backfill target을 반영한다.
- 같은 version의 changed rule content fixture가 다른 fingerprint/ID와 reservation idempotency conflict를 만든다.
- focused domain/planner/coordinator test, race detector와 전체 repository gate가 통과한다.

장기 decision은 `docs/decisions/0003-policy-identity.md`가 소유한다.
