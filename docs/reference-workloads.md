# Reference Workloads

## Purpose

이 문서는 Sema의 correctness fixture와 이후 benchmark가 반드시 포괄해야 하는 workload envelope를 정의한다. 수치 SLO는 아직 고정하지 않으며 동일 fixture로 구현 선택과 최적화를 비교한다.

## Team Versus Team

| Format | Total players | Minimum fixture variants |
|---|---:|---|
| 2:2 | 4 | all-solo, full party, mixed party |
| 3:3 | 6 | all-solo, full party, mixed party |
| 5:5 | 10 | all-solo, full party, mixed party |
| 10:10 | 20 | all-solo, full party, mixed party |
| 16:16 | 32 | all-solo, full party, mixed party |
| 20:20 | 40 | all-solo, full party, mixed party |
| 50:50 | 100 | all-solo, full party, mixed party |

`mixed party`는 각 party를 쪼개지 않고 팀 정원을 정확히 채우는 여러 크기의 ticket 조합을 뜻한다.

## Battle Royale

| Format | Total players | Ticket distribution |
|---|---:|---:|
| Duo | 100 | 2인 party 50개 |
| Squad | 100 | 4인 party 25개 |

P0에서는 homogeneous party distribution부터 검증한다. solo와 여러 party size가 섞인 100인 fixture는 남은 범위 결정 후 추가한다.

## Backfill

각 대표 크기에서 다음 상태를 재사용한다.

- 한 자리 vacancy.
- 정확히 하나의 party가 들어갈 수 있는 연속 vacancy.
- 현재 빈자리보다 큰 party만 대기 중인 no-match.
- snapshot 이후 roster version이 바뀌는 stale proposal.
- 같은 ticket을 두 session이 동시에 reserve하려는 conflict.

## Objective Schedule

1. 짧은 대기 구간에서는 skill balance와 role composition을 우선한다.
2. 대기가 늘어나면 wait time 가중치를 높이고 skill/role 허용 범위를 넓힌다.
3. 확장된 후보 안에서는 network latency가 낮은 조합을 우선한다.
4. party integrity, capacity, absolute network latency cap은 모든 구간에서 유지한다.

## Metrics To Set

- matchmaking cycle p50/p95/p99.
- maximum queue wait.
- absolute network latency cap.
- proposal당 탐색 후보 수와 CPU/memory budget.
- matched player 비율과 unmatched reason distribution.
- team skill gap과 role composition penalty.
