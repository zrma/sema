# Sema Flow TUI

## Purpose

`cmd/sema-tui`는 mixed-party ticket이 유입되고 `ProposalBatch`, reservation, assignment와 terminal acknowledgment를 거쳐 match로 이탈하는 과정을 보여주는 interactive demo다. aggregate benchmark report와 달리 lifecycle 순서와 party-preserving placement를 눈으로 확인하는 것이 목적이다.

화면 연출은 authority가 아니다. `internal/flow`가 격리된 durable journal과 loopback `v0alpha1` HTTP server를 열고, TUI는 실제 API 결과를 event로 받아 렌더링한다.

## Run

```sh
go run ./cmd/sema-tui
```

기본 workload는 deterministic seed `42`, 2x5 team, cycle당 match 2개와 반복되는 duo/solo/trio party pattern을 사용한다. 각 simulator step은 ticket ingest, plan, reserve, confirm 또는 acknowledgment 중 정확히 하나의 HTTP operation을 실행한다. lifecycle operation 사이에도 새 ticket을 넣어 다음 cycle의 대기 수요가 계속 쌓이게 한다.

```sh
go run ./cmd/sema-tui -seed 73 -interval 150ms -matches-per-cycle 3
```

## Visual Language

```text
solo    [●]
duo     [●─●]
trio    [●─●─●]
```

- `◉`: planner가 반환한 side-effect-free proposal.
- `◆`: authoritative reservation으로 잠긴 proposal.
- `✓`: assignment confirmation 또는 completed departure.
- Braille frame: HTTP lifecycle operation이 실행 중인 indeterminate activity.
- block bar: plan이 끝난 뒤 반환된 candidate/search evidence. 실행 중 진행률로 사용하지 않는다.

입력 ticket은 대기 lane을 이동하고, 실제 proposal team placement가 party glyph를 묶는다. confirmation 뒤에는 coordinator가 ticket을 소비하며 acknowledgment가 완료되면 match가 departed history로 이동한다.

## Controls

- `space`: pause/resume.
- `n`: paused 상태에서 한 lifecycle step 실행.
- `+` / `-`: 50 ms부터 2 s 사이에서 step interval 변경.
- `u`: Unicode와 ASCII compatibility glyph 전환.
- `m`: staged motion과 reduced-motion 전환.
- `q` 또는 `Ctrl-C`: 종료.

`NO_COLOR` 또는 `-no-color`는 ANSI color를 끈다. `-ascii`는 box drawing, Braille과 player glyph를 ASCII로 바꾸고 `-reduced-motion`은 중간 이동 frame을 생략한다.

## Snapshot

interactive renderer 없이 같은 fixture를 확인하려면 deterministic snapshot을 사용한다.

```sh
go run ./cmd/sema-tui -snapshot -steps 34 -width 120 -height 38
go run ./cmd/sema-tui -snapshot -ascii -steps 34 -width 100 -height 32
```

snapshot은 color와 staged motion을 끄고 terminal identity가 없는 plain text를 출력한다. 기본 34-step frame에는 이전 cycle의 departed match, 다음 cycle의 active reservation과 새 waiting ticket이 함께 나타난다.

## Truth Boundary

- TUI가 생성한 synthetic ticket만 표시하며 arbitrary external producer traffic을 관찰하지 않는다.
- planner 내부 candidate 방문을 실시간 stream하지 않는다. indeterminate activity 뒤 완료된 proposal evidence만 표시한다.
- embedded demo는 production scheduler, push delivery, authentication 또는 external allocation server가 아니다.
- raw durable payload와 실제 player identity는 화면이나 tracked fixture에 사용하지 않는다.
- interactive timing은 presentation control이며 matchmaking wait authority는 deterministic server clock이 소유한다.

실제 shared server의 전체 queue를 관찰하려면 authenticated redacted event stream과 queue read model을 별도 contract로 설계해야 한다.
