# Sema v1.0.0

## Stable Scope

This release admits the authenticated HTTP `/v1` service wire as Sema's stable public surface. The Go `github.com/zrma/sema/alpha` package, diagnostic schemas, metrics and deployment-specific configuration remain experimental or independently versioned.

Sema remains a standalone general-purpose matchmaking framework. This release does not integrate a real game service, migrate production traffic, replace an existing matcher or perform a database cutover.

## Compatibility And Support

- Same-major service-wire changes are additive and backward compatible.
- Repository maintainers own support.
- The latest stable minor and its immediate predecessor are supported for at least 180 days after the successor release.
- A deprecated route, field or command remains available for at least two subsequent minor releases and 180 days, whichever is longer.
- `/v0alpha2` is a supported compatibility alias backed by the same durable authority. Its end of support is not scheduled at `v1.0.0`.
- Only a critical authentication, tenant-isolation or data-integrity security defect can shorten this window, with an advisory, migration, rollback limitation, regression evidence and explicit end-of-support signal.

## Migration And Rollback

New clients use `/v1` and expect `api_version: "v1"`. Existing `/v0alpha2` clients can migrate by changing the route prefix without changing payloads, permissions, resource identifiers or durable data. During the support window, a client can roll back to `/v0alpha2`; clients using future `/v1`-only additive fields or endpoints must stop using them first.

See `docs/migrations/v0alpha2-to-v1.md` for the complete route mapping and rollback boundary.

## Verification

- `v0.4.0` client to `v1.0.0` service
- `v1.0.0` client in compatibility mode to `v0.4.0` service
- current `/v1` lifecycle and `/v0alpha2` shared-state regression
- canonical local, container, PostgreSQL workload/failure/recovery and publication gates
- checksummed multi-platform binaries and wire compatibility matrix

## Known Limits

- The public Go `alpha` package is not a stable Go API.
- No production-calibrated SLA, provider-specific deployment profile, managed PostgreSQL failover/PITR product contract or cross-region topology is promised.
- Token acquisition, external TLS termination and deployment inventory remain deployment responsibilities.
- Stable admission is repository-owned evidence, not proof of external game adoption or production traffic.

## Changes

- Add canonical `/v1` routes while retaining `/v0alpha2` as a bounded compatibility alias.
- Make `sema-conformance` validate `/v1` by default and keep `sema-target-smoke` on the P30 alpha baseline.
- Add the stable service-wire ADR, migration guide, numeric support policy and executable release gates.
- Preserve the tagged predecessor/current wire matrix as a checksummed release asset.
