# Sema

Sema는 플레이어와 파티를 제약 조건에 맞는 게임 세션으로 구성하는 multiplayer match composition engine이다.

이름은 서로 다른 두 영역을 하나로 묶는 고대 이집트의 [Sema-Tawy](https://www.metmuseum.org/art/collection/search/570445) 상징에서 가져왔다. 저장소 이름은 은유를 제공하지만 코드와 API는 `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`처럼 업계에서 통용되는 명시적 용어를 사용한다.

## Status

P0 architecture foundation부터 P28 matcher V0 exit까지 구현되었다. planner는 multi-proposal이 가능한 snapshot에서 diverse admissible candidate graph를 만든 뒤 ticket/backfill이 겹치지 않는 proposal 집합을 선택한다. backfill priority 안에서 policy의 `PrioritizeWait` 단계에 도달한 feasible demand는 oldest-first service ordering이 batch rank utility보다 앞서므로 지속적인 신규 유입에도 영구히 밀리지 않는다. backfill은 vacancy shape뿐 아니라 `rosterVersion`에 묶인 team별 player/skill/role/latency aggregate에 incoming party를 합친 resulting roster quality를 평가한다. 최대 12 match ticket/2 backfill/2 team의 평가 경로는 모든 admissible disjoint batch를 열거하고, default small-queue planner는 distinct ticket-set alternatives와 Pareto repair로 128-seed differential corpus에서 모두 frontier equivalent를 유지한다. large queue에는 party/skill/role/latency partition을 재사용하면서 기존 oldest-fitting prefix와 정확히 같은 결과를 내는 index seam이 있다. multi-proposal/roster-aware planner fuzz와 linear/indexed discovery fuzz를 포함한 conformance gate가 이 matcher contract를 고정한다. Flow는 5초 planning window에서 한 match 분량부터 partial batch를 허용하고 backlog가 크면 기본 상한 32개까지 한꺼번에 반환한다. deterministic composition부터 순차 queue 유입, assignment confirm, synthetic fixed-duration game, rating 기반 cooldown/복귀와 재현 가능한 wait/throughput/saturation report까지 실행 가능하다. TUI는 player-weighted queue wait와 1500 중심 rating density의 시간 변화를 함께 보여준다. 진행 중인 game 수는 관찰값일 뿐 새 planning을 제한하지 않는다. single-writer journal과 HTTP API는 현재 V0 service prototype이다. P29의 첫 slice가 tenant-scoped transactional repository contract, versioned planning snapshot, candidate-index freshness와 V0 import mapping을 추가했으며 다음 단계는 persistent adapter와 authenticated target API다. reference container SLO는 통과하지만 인증된 remote production deployment나 안정적인 SDK는 아직 아니며 v1 release는 gate가 차단한다.

## Public Contract

- Go module identity는 `github.com/zrma/sema`다.
- source는 Apache License 2.0으로 공개한다.
- `github.com/zrma/sema/alpha`만 experimental public Go package이며 현재 marker는 `v0alpha5`이고 source stability를 약속하지 않는다.
- coordinator, reservation, assignment와 나머지 구현 package는 계속 `internal/`에 둔다.
- 현재 service integration은 versioned HTTP, durable replay, synchronous response와 assignment polling을 사용하는 single replica다.

## Design Direction

- 새 매치 요청과 기존 세션의 backfill 수요를 하나의 탐색 모델에서 다룬다.
- 한 matchmaking cycle에서 개별 threshold를 통과하고 ticket이 겹치지 않는 `MatchProposal` 집합을 backfill, wait-priority service, coverage/quality 순서로 선택해 `ProposalBatch`로 반환한다.
- candidate discovery, hard constraints, scoring, proposal, reservation, commit을 분리한다.
- 탐색기는 side effect 없는 deterministic core로 유지한다.
- 결과뿐 아니라 선택 이유와 탈락 이유를 설명 가능한 evidence로 남긴다.
- 정책은 교체 가능하게 만들고 orchestration과 저장소 구현에 결합하지 않는다.

## Repository Map

- `AGENTS.md`: 짧은 AI-first bootstrap map과 GPT-5.6 baseline.
- `docs/agent-harness.md`: 자율 실행, 검증, 권한, 에스컬레이션 계약.
- `docs/HANDOFF.md`: 무컨텍스트 작업 시작점.
- `docs/status.md`: 현재 구현 상태와 리스크.
- `docs/roadmap.md`: milestone 순서와 완료 기준.
- `docs/architecture.md`: 초기 시스템 경계와 핵심 invariant.
- `docs/domain-model.md`: entity identity, freshness, ownership 계약.
- `docs/lifecycle.md`: ticket, reservation, assignment 전이와 typed failure.
- `docs/reference-workloads.md`: 대표 match size와 party distribution.
- `docs/reference-scenarios.md`: 실행 가능한 correctness와 benchmark fixture.
- `docs/sema-lab.md`: executable reference corpus와 report 사용법.
- `docs/workload-evaluation.md`: synthetic model, metric vocabulary와 bounded oracle 계약.
- `docs/evaluation-baseline.md`: deterministic coverage/search/oracle regression budget.
- `docs/matcher-v0-exit.md`: matcher completion sequence와 persistence/API productization 진입 기준.
- `docs/matcher-conformance.md`: matcher V0 invariant와 executable test matrix.
- `docs/todo-0040-service-productization-entry/spec.md`: persistence/API 제품화 milestone의 시작 계약.
- `docs/candidate-discovery.md`: candidate ticket window와 large-queue tradeoff.
- `docs/public-api.md`: public `alpha.Compose` 범위와 사용법.
- `docs/api-compatibility.md`: alpha 변경·migration과 stable API gate.
- `docs/releasing.md`: binary/module distribution과 승인 기반 release 절차.
- `docs/durable-runtime.md`: journal durability, recovery, retry와 audit 계약.
- `docs/service-api.md`: versioned ingestion, proposal authority와 assignment delivery API.
- `docs/observability.md`: health, metrics, trace와 redacted audit contract.
- `docs/operational-validation.md`: 부하, soak, restart와 torn-tail failure 검증 계약.
- `docs/operations-runbook.md`: single-writer container 배포, backup/recovery와 rollback 절차.
- `docs/performance-slo.md`: reference target profile, 반복 latency/allocation budget과 CI history.
- `docs/release-admission.md`: alpha/stable release gate와 현재 blocker.
- `docs/sema-flow.md`: 1,000명 population의 match, game result와 rating 변화를 보여주는 interactive Unicode TUI.
- `docs/sema-flow-measurement.md`: closed-loop wait, assignment yield, throughput, saturation과 quality report 계약.
- `docs/sema-flow-capacity-matrix.md`: 여러 seed와 planning batch의 동일-demand 비교 계약.
- `docs/decisions/`: 확정된 architecture decision.
- `docs/todo-*/`: 완료 evidence와 현재 milestone을 담는 작업 계약.
- `docs/REPO_MANIFEST.yaml`: repository entrypoint와 검증 명령.

## Local Verification

reference workload를 직접 실행하려면 다음 명령을 사용한다.

```sh
go run ./cmd/sema-lab -list
go run ./cmd/sema-lab team-2v2-mixed backfill-2v2-two-slots
```

public alpha consumer는 다음 명령으로 실행한다.

```sh
go run ./examples/compose
```

고정 population의 순차 유입부터 match, 동시 game, 승패와 rating 기반 cooldown/복귀까지 직접 보려면 Sema Flow를 실행한다.

```sh
go run ./cmd/sema-tui
go run ./cmd/sema-tui -snapshot -steps 100
go run ./cmd/sema-flow-report
go run ./cmd/sema-flow-report -format json -duration 10m
go run ./cmd/sema-flow-matrix
```

전체 저장소 검증은 다음과 같다.

```sh
scripts/check.sh
```

Docker image와 persistent-volume restart 검증은 Docker daemon이 있는 환경에서 별도로 실행한다.

```sh
scripts/check-container.sh
```

로컬 change 관리는 `jj`를 사용한다. push, tag, release와 visibility 변경은 별도 외부-write 권한 경계다.

## License

Apache License 2.0. 자세한 내용은 `LICENSE`를 참고한다.
