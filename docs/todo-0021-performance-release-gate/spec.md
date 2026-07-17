# P10 Performance And Release Admission Spec

- Status: Complete

## Objective

reference container에서 반복 latency/recovery/allocation budget을 실행하고 alpha/stable release channel을 version-aware gate로 보호한다.

## Deliverables

- sanitized Go benchmark parser와 numeric budget report.
- 2 CPU/2 GiB repeated container service SLO.
- CI performance history artifact.
- full/container/performance/recovery를 묶은 release admission command.
- explicit alpha admission과 stable-blocked machine-readable state.

## Acceptance

- 네 representative benchmark가 최소 3 samples의 worst ns/B/alloc budget을 만족한다.
- 3 service runs가 lifecycle count, metrics, restart/torn-tail recovery와 latency budget을 만족한다.
- artifact에 raw CPU/host/path/resource identity가 없다.
- v0 admission이 모든 local gate를 실행한다.
- v1 admission이 current blocker 때문에 artifact publication 전에 실패한다.
- full repository/container/performance/publication gate가 통과한다.

## Out Of Scope

- production SLA와 actual consumer traffic calibration.
- stable API/transport 구현 또는 stable release publication.
- public container registry, signing/SBOM/provenance publication.
- multi-replica database benchmark와 online backup SLO.

## Completion Evidence

`scripts/check-performance.sh`와 `scripts/check-release-admission.sh v0.0.0-test`가 통과하고 `v1.0.0`은 stable blocker로 실패한다. profile/budget은 `docs/performance-slo.md`, channel gate는 `docs/release-admission.md`, decision은 ADR 0015가 소유한다.
