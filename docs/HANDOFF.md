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
- public `alpha.Compose`가 explicit public/internal copy boundary로 immutable composition을 제공한다.
- `examples/compose`가 `internal/` import 없이 alpha integration을 실행한다.
- alpha compatibility/migration gate와 versioned `sema-lab` binary/checksum release workflow가 준비되어 있다.
- `internal/durable`이 checksummed journal sync 뒤 성공을 반환하고 restart 때 reservation/assignment/idempotency를 replay한다.
- complete plan audit와 snapshot ID idempotency, torn-tail recovery, corruption refusal, fixed TTL과 single-writer lock이 P9 persistence authority를 고정한다.
- `cmd/sema-server`가 explicit `v0alpha1` DTO로 ingestion부터 assignment poll/ack까지 제공한다.
- HTTP server clock과 durable proposal ID lookup이 TTL manipulation과 forged placement를 service boundary에서 차단한다.
- `/metrics`, W3C request trace, liveness/readiness와 redacted paged audit가 bounded operational evidence를 제공한다.
- `cmd/sema-ops-check`가 실제 HTTP lifecycle 부하, 완료 assignment restart replay와 incomplete journal tail 복구를 격리된 임시 runtime에서 검증한다.
- `Dockerfile`과 loopback-only Compose example이 non-root/read-only/capability-free single-writer deployment를 제공하고 operations runbook이 offline backup/restore를 고정한다.
- reference container profile이 repeated service latency/recovery와 planner/engine/replay allocation budget을 검증하고 CI가 redacted aggregate history를 보존한다.
- release admission은 v0 alpha만 허용하며 stable API/transport/consumer evidence 전에는 v1 tag를 차단한다.
- `cmd/sema-tui`가 실제 loopback HTTP lifecycle 위에서 empty queue로 시작하는 mixed-party arrival, proposal/reservation, concurrent game과 completion을 Unicode animation으로 보여준다.
- Flow snapshot과 ASCII/reduced-motion fallback이 terminal-independent self-check를 제공하며 demo timing은 production scheduler authority가 아니다.
- 기본 1,000명 closed population registry가 fixed party로 순차 유입되고 45초 game을 반복하며 hidden true skill 기반 승패 뒤 visible Elo rating을 갱신한다.
- completed party는 분산 cooldown 뒤 증가한 revision과 새 rating을 가진 동일 ticket으로 실제 HTTP 복귀하고 TUI는 lifecycle population, rating 분포와 최근 result를 표시한다.
- `scripts/check.sh`가 Go format, vet, test, race detector, reference benchmark와 repository gate를 실행한다.
- repository identity는 `github.com/zrma/sema`이고 source는 Apache-2.0으로 공개한다.
- `alpha` 외 Go package는 `internal/`에 유지하며 stable API와 wire compatibility는 아직 제공하지 않는다.
- numeric SLO, skill metric, role schema와 multi-replica persistence는 아직 결정하지 않았다.
- publication class는 `public`이며 push 전 repository gate와 machine-local inventory gate를 모두 통과한다.

## Current Work

P0 foundation부터 P12 closed-loop population simulation까지 완료되었다. planner/coordinator/journal은 한 writer에 유지하고 Flow의 game/result model은 synthetic reference workload로만 둔다. 다음 milestone은 실제 evaluation metric이나 consumer/target input이 생길 때 열며 stable v1은 현재 차단되어 있다.

## Completion Rule

분석이나 patch 적용만으로 완료하지 않는다. acceptance에 대응하는 fixture/test와 전체 local gate를 통과하고 status/roadmap을 현재 상태에 맞춘다. push, tag, release, visibility 변경은 별도 권한 경계다.
