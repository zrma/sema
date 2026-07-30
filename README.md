# Sema

Sema는 플레이어와 파티를 제약 조건에 맞는 게임 세션으로 구성하는 multiplayer match composition engine이다.

이름은 서로 다른 두 영역을 하나로 묶는 고대 이집트의 [Sema-Tawy](https://www.metmuseum.org/art/collection/search/570445) 상징에서 가져왔다. 저장소 이름은 은유를 제공하지만 코드와 API는 `MatchTicket`, `BackfillTicket`, `ProposalBatch`, `MatchProposal`, `Reservation`, `Assignment`처럼 업계에서 통용되는 명시적 용어를 사용한다.

## Status

P0부터 P28 matcher V0 exit, P29 service productization entry와 P30 authenticated service runtime까지 완료되었다. planner는 immutable snapshot에서 다양한 admissible match candidate를 만들고, ticket/backfill이 겹치지 않는 proposal 집합을 wait-priority, coverage와 quality 순서로 선택한다. Flow/TUI는 1,000명 closed population의 순차 유입, match, synthetic game, rating 변화와 재진입을 보여주지만 game simulation은 Sema service의 책임이 아니다.

P31에서는 PostgreSQL primary와 stateless service replica를 표준 runtime으로 승격했다. `sema-conformance`가 OIDC 인증 실패, tenant isolation과 complete lifecycle을, two-replica matrix가 reservation single-winner, restart와 PostgreSQL outage/recovery를 검증한다. `sema-standard-postgres-v1`은 same-tenant concurrent commit deadlock 회귀를 차단하고 64-request admission, 16/2 PostgreSQL pool, 5초 operation deadline과 p95 750ms reference regression budget을 반복 측정한다. 표준 runtime은 bounded Prometheus metric/redacted trace/reference alert를 제공하고 native PostgreSQL checkpoint를 destructive restore한 뒤 operation replay, terminal assignment와 새 API write까지 검증한다.

Sema는 기존 배포나 실제 게임 트래픽을 이전하는 프로젝트가 아니라 standalone general-purpose matchmaking service다. P31 product-readiness evidence는 완료되었다. P30 `v0alpha2` wire가 첫 compatibility baseline이므로 stable surface, tagged multi-version matrix와 deprecation/support 약속을 별도 승인하기 전까지 v1 release는 gate가 차단한다.

## Public Contract

- Go module identity는 `github.com/zrma/sema`다.
- source는 Apache License 2.0으로 공개한다.
- `github.com/zrma/sema/alpha`만 experimental public Go package이며 현재 marker는 `v0alpha5`이고 source stability를 약속하지 않는다.
- coordinator, reservation, assignment와 나머지 구현 package는 계속 `internal/`에 둔다.
- 현재 standard service는 versioned HTTP, PostgreSQL durable replay, synchronous response와 assignment polling을 사용하는 stateless replica다.

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
- `docs/todo-0042-service-product-readiness/spec.md`: 완료된 표준 runtime, compatibility와 operational evidence milestone.
- `docs/repository-adapter-evidence.md`: persistent prototype의 crash/contention evidence와 storage decision outcome.
- `docs/postgres-repository.md`: PostgreSQL schema, transaction, migration과 no-Redis 운영 경계.
- `docs/oidc-authentication.md`: provider-neutral JWT claim과 permission mapping 계약.
- `docs/remote-runtime.md`: authenticated PostgreSQL executable, migration, TLS와 deployment acceptance.
- `docs/wire-conformance.md`: 표준 reference client, provider-neutral lifecycle fixture와 report contract.
- `docs/runtime-failure-matrix.md`: two-replica contention, restart와 PostgreSQL outage/recovery evidence.
- `docs/service-workload.md`: 표준 PostgreSQL service의 admission, pool, timeout과 numeric regression profile.
- `docs/postgres-recovery.md`: native PostgreSQL checkpoint restore와 deployment PITR acceptance contract.
- `docs/wire-compatibility.md`: P30 wire baseline, alpha deprecation policy와 stable admission requirement.
- `docs/v0-runtime.md`: V0 single-writer journal development/reference 및 import compatibility runbook.
- `docs/candidate-discovery.md`: candidate ticket window와 large-queue tradeoff.
- `docs/public-api.md`: public `alpha.Compose` 범위와 사용법.
- `docs/api-compatibility.md`: alpha 변경·migration과 stable API gate.
- `docs/releasing.md`: binary/module distribution과 승인 기반 release 절차.
- `docs/durable-runtime.md`: journal durability, recovery, retry와 audit 계약.
- `docs/service-api.md`: versioned ingestion, proposal authority와 assignment delivery API.
- `docs/observability.md`: health, metrics, trace와 redacted audit contract.
- `docs/operational-validation.md`: 부하, soak, restart와 torn-tail failure 검증 계약.
- `docs/operations-runbook.md`: 표준 PostgreSQL/OIDC container 배포, failure와 recovery 절차.
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

Docker image의 표준 entrypoint와 V0 persistent-volume restart compatibility 검증은 Docker daemon이 있는 환경에서 별도로 실행한다.

```sh
scripts/check-container.sh
```

표준 PostgreSQL service의 workload, multi-replica failure와 logical recovery gate는 test 전용 disposable database에서 실행한다.

```sh
scripts/check-postgres.sh
```

로컬 change 관리는 `jj`를 사용한다. push, tag, release와 visibility 변경은 별도 외부-write 권한 경계다.

## License

Apache License 2.0. 자세한 내용은 `LICENSE`를 참고한다.
