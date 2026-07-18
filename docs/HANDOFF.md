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
- planner는 greedy cover와 anchored search로 admissible candidate graph를 만들고 backfill-first, oldest wait-priority service, quality ordering으로 deterministic disjoint batch를 선택한다. coordinator는 revision CAS와 fixed-TTL reservation/assignment를 소유한다.
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
- P23 evaluation이 12 match ticket/2 backfill/2 team 이하의 모든 admissible disjoint batch를 열거해 planner의 coverage/wait/per-match quality Pareto relation과 dominating witness를 제공한다.
- P24 planner가 candidate budget을 명시하지 않은 같은 small boundary에서 ticket-set alternatives와 Pareto subset repair를 사용하며 128-seed differential corpus가 모두 frontier equivalent임을 검증한다.
- P25 planner가 각 match/backfill demand의 priority age를 계산하고 backfill tier 안에서 oldest eligible wait를 rank utility보다 먼저 service한다. sustained fresh-arrival fixture는 configured 30초 경계에서 오래된 pair와 priority evidence를 고정한다.
- P26 backfill이 optional roster-versioned team aggregate와 incoming placement의 resulting skill/role/latency를 평가하고 exhaustive frontier 및 stale reserve fixture가 같은 freshness contract를 검증한다.
- P27 reusable discovery index가 party/skill/role/latency partition을 canonical age order로 merge해 96-ticket matrix와 10K queue에서 linear oldest-prefix와 exact-equivalent window를 만든다. per-plan build는 하지 않고 stateful owner 연결을 productization entry에 둔다.
- P28 conformance matrix가 planner의 multi-proposal/backfill fuzz, discovery exact-equivalence fuzz, exhaustive frontier, fairness와 freshness fixture를 하나의 matcher V0 exit gate로 묶는다.
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
- `cmd/sema-flow-report`가 같은 closed loop를 headless로 실행하고 player-weighted wait, assignment yield, match throughput, time-weighted queue saturation과 proposal quality를 versioned aggregate로 출력한다.
- Flow scheduler는 presentation frame과 logical clock을 분리하고 due ingress, batch reservation/confirmation, completion과 planning을 deterministic timestamp 순서로 처리한다.
- `sema.flow.measurement.v0alpha3`가 ingress arrival lag와 horizon backlog를 노출하며 frontend-owned game-capacity field를 포함하지 않는다.
- active game 수는 planning eligibility를 제한하지 않는다. Flow는 assignment confirm 이후 game/result/return을 synthetic하게 모사하고 TUI `MATCH LIFECYCLE` 패널에서 계속 보여준다.
- `cmd/sema-flow-matrix`가 seed 42/73/101과 planning upper bound 2/8/32를 비교하고 throughput, wait, queue와 quality min/median/max를 `sema.flow.capacity-matrix.v0alpha2`로 출력한다.
- wide Flow TUI는 `WAITING POOL | MATCH LIFECYCLE`, `AVERAGE QUEUE WAIT | RATING DENSITY`, `COMPLETED MATCHES | EVENT STREAM`의 세 행을 사용한다.
- trend는 player-weighted pre-confirm wait와 1500-centered whole-population rating density를 최근 512개의 10초 logical-time bucket으로 보여준다. 신규 bucket은 기존 열을 재투영하지 않고 값 그대로 왼쪽으로 이동시킨다.
- selected party row는 match별 marker/color를 공유해 hold와 horizontal departure를 거친 뒤 제거되며, 남은 waiting row는 frame 단위로 위로 접힌다. reduced-motion은 동일 final state를 즉시 적용한다.
- 새 lifecycle block은 같은 marker/color를 유지한 채 batch 순서대로 panel 상단에서 stagger되어 펼쳐지고, 기존 block은 드러난 row만큼 아래로 이동한다. 이 motion도 presentation-only이며 reduced-motion은 최종 배치를 즉시 표시한다.
- rating density는 exact 1500과 양쪽 25점 histogram에서 visible history의 대칭 Y축 범위를 고르고 최대 9개 band를 analytics panel 높이에 비례해 확장한다. 반복 row는 시각적 cell height이지 추가 rating data가 아니다.
- P18 global selector는 `MaxProposals`를 상한으로 사용하고 candidate/selection budget을 분리하며, best feasible batch와 rank utility/truncation evidence를 public alpha, HTTP DTO와 durable replay에 보존한다.
- Flow는 5v5 한 match 분량부터 partial batch를 계획하고 backlog가 있으면 기본 32-match upper bound까지 한 cycle에 반환한다. 400-player fixture는 한 cycle 32 proposals를 고정하고 1,000-player 정상상태 구간은 89.9 match/min을 기록했다.
- selector cardinality가 하나이면 anchored batch alternative를 생략하는 P20 fast path가 50v50, 100K queue와 engine 1,000-ticket을 기존 reference performance budget 안에 유지한다. multi-proposal 또는 backfill 경쟁 경로는 P18 candidate graph를 유지한다.
- `scripts/check.sh`가 Go format, vet, test, race detector, reference benchmark와 repository gate를 실행한다.
- repository identity는 `github.com/zrma/sema`이고 source는 Apache-2.0으로 공개한다.
- `alpha` 외 Go package는 `internal/`에 유지한다. public Go marker는 P26 roster-aware backfill migration을 반영한 `v0alpha5`이며 stable API와 wire compatibility는 아직 제공하지 않는다.
- numeric SLO, skill metric, role schema와 multi-replica persistence는 아직 결정하지 않았다.
- publication class는 `public`이며 push 전 repository gate와 machine-local inventory gate를 모두 통과한다.

## Current Work

P0 foundation부터 P28 matcher V0 exit까지 완료되었다. planner/coordinator/journal은 한 writer에 유지하고 Flow의 game/result/measurement/matrix/trend model은 synthetic reference workload로만 둔다. Sema는 assignment confirm까지 소유하며 frontend game execution은 planning capacity gate가 아니다. 현재 active milestone은 P29 service productization entry다. `docs/todo-0040-service-productization-entry/spec.md`에서 adapter-neutral repository contract, authority/failure matrix, stateful index lifetime과 target API resource를 먼저 정의한다. traffic calibration 없는 frontier, roster aggregate와 synthetic priority boundary는 production quality/SLA 주장이 아니며 stable v1은 현재 차단되어 있다.

## Completion Rule

분석이나 patch 적용만으로 완료하지 않는다. acceptance에 대응하는 fixture/test와 전체 local gate를 통과하고 status/roadmap을 현재 상태에 맞춘다. push, tag, release, visibility 변경은 별도 권한 경계다.
