# P4 Policy Catalog Spec

- Status: Planned

## Objective

same-process runtime에서 policy version이 하나의 canonical fingerprint만 가리키게 한다. first registration 이후 같은 version의 다른 content를 거부해 version label을 안정적인 planning contract로 만든다.

## Deliverables

- in-memory policy catalog의 register/read contract.
- same version/same fingerprint idempotent registration과 same version/different fingerprint conflict.
- engine planning이 catalog에 등록된 exact policy만 사용하도록 하는 application boundary.
- policy version별 format fixture와 process restart boundary 문서화.

## Acceptance

- first registration은 defensive policy copy와 fingerprint를 저장한다.
- repeated identical registration은 같은 read model을 반환한다.
- 같은 version의 다른 content는 typed `PolicyConflict` 결과이며 기존 policy를 변경하지 않는다.
- plan은 registered version의 exact content를 사용하고 caller mutation이 catalog에 영향을 주지 않는다.
- focused policy/engine test, race detector와 전체 repository gate가 통과한다.

## Out Of Scope

- durable policy registry와 process restart recovery.
- remote distribution, authorization과 rollout percentage.
- public schema/version negotiation과 migration.
- policy activation schedule와 multi-tenant ownership.
