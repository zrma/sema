# P10 Container And Operations Spec

- Status: Complete

## Objective

P9 single-writer service를 안전한 기본값의 container artifact로 만들고 배포, health, backup, recovery, upgrade와 rollback 운영 절차를 명시한다.

## Deliverables

- pinned multi-stage non-root `scratch` image.
- in-image readiness probe와 operational validator.
- host-loopback-only Compose deployment example.
- single-writer backup/restore/failure/upgrade runbook.
- actual image build, hardening, readiness와 volume restart gate.

## Acceptance

- image가 exact builder digest와 embedded version을 사용한다.
- runtime identity가 non-root이고 read-only root/capability drop으로 실행된다.
- durable volume restart 뒤 service가 ready 상태로 복구된다.
- deployment example이 authentication 없는 port를 non-loopback host에 publish하지 않는다.
- runbook이 replica 1, offline backup, complete corruption refusal와 fixed TTL을 보존한다.
- container gate와 full repository/publication gate가 통과한다.

## Out Of Scope

- public image push, signing, SBOM과 provenance publication.
- built-in authentication/TLS/rate limit.
- Kubernetes HA, multi-replica와 online backup.
- stable API/release 선언.

## Completion Evidence

`scripts/check-container.sh`가 image build, in-image operational lifecycle와 volume-backed restart를 통과한다. 운영 계약은 `docs/operations-runbook.md`, architecture decision은 ADR 0014가 소유한다.
