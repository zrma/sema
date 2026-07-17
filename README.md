# Sema

Sema는 플레이어와 파티를 제약 조건에 맞는 게임 세션으로 구성하는 multiplayer match composition engine이다.

이름은 서로 다른 두 영역을 하나로 묶는 고대 이집트의 [Sema-Tawy](https://www.metmuseum.org/art/collection/search/570445) 상징에서 가져왔다. 저장소 이름은 은유를 제공하지만 코드와 API는 `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`처럼 업계에서 통용되는 명시적 용어를 사용한다.

## Status

P0 architecture foundation부터 P15 multi-seed Flow capacity matrix까지 구현되었다. deterministic composition부터 순차 queue 유입, 동시 fixed-duration game, terminal assignment acknowledgment, rating 기반 cooldown/복귀와 재현 가능한 wait/throughput/saturation report까지 실행 가능하며 logical timestamp scheduler가 TUI frame 수와 workload admission을 분리한다. multi-seed matrix는 동일 demand에서 concurrency profile별 변동 범위를 비교한다. single-writer journal은 reservation, assignment와 decision audit을 재시작 뒤 복구한다. reference container SLO는 통과하지만 인증된 remote production deployment나 안정적인 SDK는 아직 아니며 v1 release는 gate가 차단한다.

## Public Contract

- Go module identity는 `github.com/zrma/sema`다.
- source는 Apache License 2.0으로 공개한다.
- `github.com/zrma/sema/alpha`만 experimental public Go package이며 `v0alpha1` source stability를 약속하지 않는다.
- coordinator, reservation, assignment와 나머지 구현 package는 계속 `internal/`에 둔다.
- 현재 service integration은 versioned HTTP, durable replay, synchronous response와 assignment polling을 사용하는 single replica다.

## Design Direction

- 새 매치 요청과 기존 세션의 backfill 수요를 하나의 탐색 모델에서 다룬다.
- 한 matchmaking cycle에서 ticket이 겹치지 않는 여러 `MatchProposal`을 `ProposalBatch`로 반환한다.
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
- `docs/sema-flow-capacity-matrix.md`: 여러 seed와 concurrency profile의 동일-demand 비교 계약.
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
