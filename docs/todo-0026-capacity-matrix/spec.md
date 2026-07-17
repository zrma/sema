# P15 Multi-Seed Capacity Matrix Spec

- Status: Superseded by P16

이 milestone은 실행 framework와 동일-demand comparability를 확립했지만 frontend가 소유하는 concurrent game capacity와 Sema가 소유하는 planning batch를 같은 profile에 섞었다. historical completion evidence는 보존하되 현재 profile 계약과 reference result는 `docs/todo-0027-unbounded-game-simulation/spec.md`가 대체한다.

## Objective

동일한 closed-population demand horizon에서 여러 batch/concurrency profile을 여러 deterministic seed로 실행하고, throughput, wait, queue pressure와 quality 변동 범위를 한 report로 비교한다. P14의 ingress comparability contract를 모든 run의 선행 gate로 사용한다.

## Reference Matrix

- simulated duration: 10 minutes.
- seeds: `42`, `73`, `101`.
- profiles: concurrent/batch `8/2`, `16/4`, `32/8`.
- 각 profile/seed 조합은 독립 Flow runtime에서 실행한다.
- wall-clock parallelism은 결과 순서나 aggregate에 영향을 주지 않는다.

## Report Contract

- JSON schema는 `sema.flow.capacity-matrix.v0alpha1`이다.
- profile별 run 수와 assignment yield, confirmed/completed throughput, wait p50/p90, queue mean/p95, skill-gap p90을 min/median/max로 집계한다.
- initial ticket count, maximum arrival lag와 final ingress backlog도 같은 방식으로 집계한다.
- `demand_comparable`은 같은 seed의 모든 profile이 같은 initial ticket 수를 관측하고 모든 run의 arrival lag와 final backlog가 0일 때만 true다.
- text output은 deterministic profile order와 compact range vocabulary를 사용하고 JSON은 raw player/ticket identity나 wall-clock timing을 포함하지 않는다.

## Acceptance

- seed/profile parsing은 중복, 음수 seed, malformed profile과 invalid concurrency를 거부한다.
- worker parallelism과 무관하게 같은 input의 report가 byte-equivalent하다.
- aggregate ordering과 even/odd median rule이 fixture로 고정된다.
- demand mismatch, arrival lag 또는 final backlog는 `demand_comparable=false`로 보존된다.
- 축소된 실제 Flow matrix가 focused/race test에서 실행된다.
- 기본 reference matrix를 실행하고 결과를 `docs/sema-flow-capacity-matrix.md`에 기록한다.
- `scripts/check.sh`와 publication boundary gate를 통과한다.

## Truth Boundary

- matrix는 synthetic single-process Flow의 configuration sensitivity다.
- min/median/max는 세 deterministic seed의 범위이며 confidence interval이 아니다.
- traffic target, queue SLA나 cost function이 없으므로 profile을 자동 추천하거나 production capacity를 선언하지 않는다.
- 실제 producer arrival, permanent churn, external allocator, multi-replica coordination과 infrastructure saturation은 범위 밖이다.

## Completion Evidence

- fake measurer fixture가 parallelism 1/4에서 같은 report, canonical ordering, min/median/max와 failure propagation을 검증한다.
- parser/CLI fixture가 malformed input을 거부하고 40-player 실제 Flow 2-seed/2-profile matrix에서 동일 demand를 확인한다.
- 기본 3x3 run은 모든 조합에서 initial ticket 600개, arrival lag 0ms와 final ingress backlog 0을 기록했다.
- 기본 profile의 median confirmed throughput은 10,000 / 18,700 / 32,700 milli-match/min, wait p50은 117,000 / 64,000 / 26,000ms다.
- focused/race/full repository gate와 publication boundary를 통과하고 결과를 `docs/sema-flow-capacity-matrix.md`에 기록한다.
