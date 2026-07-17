# ADR 0014: Single-Writer Hardened Container Baseline

- Status: Accepted

## Context

P10 operational validator는 process와 journal contract를 검증하지만 deployable artifact, writable path와 shutdown/backup 절차가 정의되지 않았다. service에는 built-in authentication/TLS가 없고 durable runtime은 local file lock 기반 single writer이므로 일반적인 replicated service manifest를 제공하면 안전하지 않은 운영 모델을 암시한다.

## Decision

- multi-stage image는 pinned Go builder에서 static server, healthcheck와 operational validator를 만들고 `scratch` runtime을 사용한다.
- runtime은 numeric non-root identity, read-only root filesystem, capability drop와 no-new-privileges를 지원한다.
- durable volume `/var/lib/sema`와 bounded `/tmp`만 writable하다.
- image default는 loopback listener이며 compose example은 host loopback에만 publish한다.
- non-loopback remote deployment는 external authentication/TLS/rate-limit gateway와 private network 승인이 없으면 지원하지 않는다.
- replica는 1이고 backup/restore는 graceful stop 뒤 offline volume operation만 허용한다.
- container gate는 in-image lifecycle, readiness와 persistent-volume restart를 실제 Docker daemon에서 실행한다.

## Consequences

- minimal runtime image와 explicit writable/state boundary를 얻는다.
- container platform에서도 journal sync, file lock과 fixed TTL semantics를 그대로 유지한다.
- image 자체만으로 remote production ingress를 제공하지 않는다.
- online backup, rolling update, horizontal availability와 orchestration manifest는 multi-writer authority가 생기기 전까지 제공하지 않는다.

## Revisit Triggers

- authenticated service transport 또는 approved gateway contract가 repository-owned surface가 된다.
- database-backed lease/transaction authority가 replica 2 이상을 허용한다.
- online snapshot/compaction과 journal schema migration이 구현된다.
- signed image publication과 provenance가 release scope에 포함된다.
