# Sema Handoff

## Start Here

1. `AGENTS.md`와 `docs/agent-harness.md`를 읽는다.
2. `jj status`로 기존 변경과 현재 change를 확인한다.
3. `docs/status.md`, `docs/roadmap.md`, `docs/architecture.md`를 읽는다.
4. 활성 `docs/todo-*/spec.md`의 objective, acceptance, out-of-scope를 고정한다.
5. focused validation 뒤 `scripts/check.sh`로 닫는다.

## Current Baseline

- 저장소 이름과 제품 방향은 Sema로 확정했다.
- `agent-harness-v1`과 GPT-5.6 baseline을 적용했다.
- new match와 backfill을 함께 다루는 초기 architecture boundary를 문서화했다.
- 구현 baseline은 Go 단일 프로세스와 인메모리 상태다.
- 한 cycle은 서로 ticket이 겹치지 않는 여러 match를 `ProposalBatch`로 반환한다.
- 대표 workload는 2:2부터 50:50까지의 team match와 총원 100명의 duo/squad battle royale이다.
- `internal/domain`, `internal/planner`, `internal/coordinator`에 P0 Go vertical slice가 구현되어 있다.
- planner는 backfill-first bounded enumeration과 deterministic disjoint batch를 만들고 coordinator는 revision CAS와 fixed-TTL reservation/assignment를 소유한다.
- role/skill quality는 versioned wait relaxation에 따라 best-known candidate를 비교하며 unmatched ticket에는 stable reason이 남는다.
- assignment는 외부 consumer의 complete/cancel/fail acknowledgment와 backfill roster CAS evidence를 idempotent하게 기록한다.
- `internal/engine`이 ingestion부터 terminal assignment까지 transport-neutral application boundary를 제공한다.
- 첫 integration은 같은 process의 `internal/engine` direct call이며 producer replay, synchronous acknowledgment, single-replica contract를 사용한다.
- full engine benchmark와 expiry/concurrency/restart fixture가 runtime evidence를 제공한다.
- coordinator의 player ownership index가 queue-wide duplicate scan 없이 active ticket uniqueness를 유지한다.
- canonical policy fingerprint와 content-aware proposal ID가 replay identity를 실제 rule content에 연결한다.
- process-local policy catalog가 explicit registration 뒤 version-only planning을 제공한다.
- offline simulation이 policy/scenario 순서와 무관한 canonical comparison report를 만든다.
- `cmd/sema-lab`이 2:2부터 50:50, 100-player battle royale, backfill/no-match와 objective fixture를 실행하고 ticket/player coverage 및 search evidence를 출력한다.
- P6 evaluation이 seeded synthetic snapshot, coverage basis points, oldest unmatched wait와 12-ticket exhaustive new-match oracle를 제공한다.
- P7 discovery가 versioned oldest-fitting ticket window, 10K correctness, 10K/100K manual benchmark와 fuzz invariant를 제공한다.
- `scripts/check.sh`가 Go format, vet, test, race detector, reference benchmark와 repository gate를 실행한다.
- repository identity는 `github.com/zrma/sema`이고 source는 Apache-2.0으로 공개한다.
- Go package는 `internal/`에 유지하며 public API와 compatibility guarantee는 아직 제공하지 않는다.
- numeric SLO, skill metric, role schema, production persistence는 아직 결정하지 않았다.
- publication class는 `public`이며 push 전 repository gate와 machine-local inventory gate를 모두 통과한다.

## Current Work

P0 foundation부터 P7 scalable candidate discovery까지 완료되었다. ADR 0006의 engine-first 순서에 따라 다음 repo-owned 작업은 P8 reference consumer와 최소 `v0alpha` integration contract다. production consumer 또는 수치 SLO가 생기기 전에는 protocol, database나 stable SDK compatibility를 추가하지 않는다.

## Completion Rule

분석이나 patch 적용만으로 완료하지 않는다. acceptance에 대응하는 fixture/test와 전체 local gate를 통과하고 status/roadmap을 현재 상태에 맞춘다. push, tag, release, visibility 변경은 별도 권한 경계다.
