# Sema Flow TUI

## Purpose

`cmd/sema-tui`는 고정된 player registry가 순차적으로 대기열에 들어오고, 5v5 matchmaking, 일정 시간의 경기, rating 갱신, cooldown과 복귀를 반복하는 interactive closed-loop simulation이다. aggregate benchmark report와 달리 party-preserving placement, assignment lifecycle과 rating 분포의 변화를 눈으로 확인하는 것이 목적이다.

화면 연출은 authority가 아니다. `internal/flow`가 격리된 durable journal과 loopback `v0alpha1` HTTP server를 열고, TUI는 실제 plan, reserve, confirm, acknowledgment와 revised ticket ingestion 결과를 event로 받아 렌더링한다.

## Run

```sh
go run ./cmd/sema-tui
```

기본 workload는 다음과 같다.

- seed `42`로 생성한 player 1,000명과 stable mixed-party 600개. 시작 시 matchmaking queue는 비어 있다.
- 모든 player의 초기 visible rating 1500과 서로 다른 hidden true skill.
- 2x5 team, cycle당 proposal 2개와 최대 동시 match 8개.
- party의 최초 queue 유입 간격 1초, planning 간격 5초.
- simulated game duration 45초, 완료 뒤 최대 복귀 지연 30초, Elo K-factor 32.
- solo/duo/trio party pattern `[2, 1, 1, 1, 3, 2]`의 반복.

빠르게 여러 rating cycle을 보려면 step interval과 동시 match 수를 높인다.

```sh
go run ./cmd/sema-tui \
  -interval 50ms \
  -matches-per-cycle 8 \
  -concurrent-matches 32 \
  -arrival-interval 500ms
```

축소 population을 확인할 수도 있다.

```sh
go run ./cmd/sema-tui -population 100 -concurrent-matches 8 -game-duration 30s
```

`-arrival-interval`, `-planning-interval`, `-game-duration`, `-max-return-delay`로 각각 최초 유입, planning cadence, 경기 시간과 복귀 대기를 조정한다. `-interval`은 simulated time이 아니라 화면에서 lifecycle step을 진행하는 속도다.

## Simulation Model

각 player는 stable identity, visible rating, hidden true skill, games와 wins를 가진다. 초기 rating은 모두 1500이지만 true skill은 600부터 2400 사이의 seeded normal-like distribution으로 생성한다. matchmaking ticket에는 visible rating만 기록하므로 planner는 true skill을 알지 못한다.

승패 확률은 양 팀의 평균 true skill 차이에 logistic curve를 적용해 계산하고 seeded draw로 승자를 정한다. 승패가 확정되면 양 팀의 평균 visible rating으로 Elo expectation을 계산하고 각 player에게 같은 team delta를 적용한다. 동일 인원의 5v5에서는 두 팀 delta가 zero-sum이며 rating은 100부터 3000 사이로 제한한다.

confirm된 assignment는 즉시 완료되지 않고 fixed game duration 동안 in-game 상태에 머문다. 시간이 끝나면 completed acknowledgment를 실제 HTTP endpoint에 기록하고, 참가 party는 새 rating과 증가한 revision을 가진 동일 ticket identity로 다시 대기한다.

population 1,000명은 identity와 rating을 소유하는 registry 크기이며 queue 크기가 아니다. 시작 상태는 전원 `idle`이고 stable party가 `arrival-interval`마다 하나씩 실제 HTTP ticket으로 queue에 들어온다. planner, reservation과 confirm이 진행되는 동안에도 due arrival을 lifecycle operation과 번갈아 처리하므로 여러 game이 동시에 진행되는 사이 queue가 계속 변한다.

한 game이 끝난 party는 즉시 일괄 재큐잉되지 않는다. 같은 seed, party identity와 revision으로 정한 복귀 지연에 따라 20%는 즉시, 50%는 5–15초, 30%는 20초부터 `max-return-delay` 사이의 `cooldown`을 거친다. 지연이 끝나면 최신 rating과 증가한 revision으로 queue에 복귀한다. 모든 시점에 `idle + queued + in-game + cooldown = population`을 유지한다.

## Visual Language

```text
solo    [●]
duo     [●─●]
trio    [●─●─●]
```

- `◉`: planner가 반환한 side-effect-free proposal.
- `◆`: authoritative reservation으로 잠긴 proposal.
- `✓ PLAYING`: assignment가 confirm되어 game timer가 진행 중인 match.
- completed row: 승리 팀, 실제 승자의 사전 확률, 양 팀 rating과 Elo delta.
- status row: 아직 최초 유입 전인 `idle`, 현재 `queued`, `in-game`, 복귀 대기 중인 `cooldown` player 수.
- rating sparkline: 전체 population의 fixed rating bucket 분포.
- Braille frame: HTTP lifecycle operation이 실행 중인 indeterminate activity.
- block bar: plan이 끝난 뒤 반환된 candidate/search evidence. 실행 중 진행률로 사용하지 않는다.
- terminal 높이가 커지면 waiting/lifecycle, completed match와 event stream panel이 남은 세로 공간을 비례 배분해 전체 화면을 사용한다.

## Controls

- `space`: pause/resume.
- `n`: paused 상태에서 한 lifecycle step 실행.
- `+` / `-`: 50 ms부터 2 s 사이에서 step interval 변경.
- header의 `speed N×`는 최근 simulated-time step과 현재 presentation interval의 비율이며 `+` / `-` 입력 즉시 갱신된다. 기본값은 `4.5×`다.
- `u`: Unicode와 ASCII compatibility glyph 전환.
- `m`: staged motion과 reduced-motion 전환.
- `q` 또는 `Ctrl-C`: 종료.

`NO_COLOR` 또는 `-no-color`는 ANSI color를 끈다. `-ascii`는 box drawing, Braille, histogram과 player glyph를 ASCII로 바꾸고 `-reduced-motion`은 중간 이동 frame을 생략한다.

## Snapshot

interactive renderer 없이 같은 fixture를 확인하려면 deterministic snapshot을 사용한다.

```sh
go run ./cmd/sema-tui -snapshot -steps 100 -width 120 -height 38
go run ./cmd/sema-tui -snapshot -ascii -population 40 \
  -concurrent-matches 4 -game-duration 20s -max-return-delay 10s \
  -steps 80 -width 100 -height 32
```

snapshot은 color와 staged motion을 끄고 terminal identity가 없는 plain text를 출력한다. 기본 frame에는 waiting population, in-game match, completed result와 벌어진 rating range가 함께 나타난다.

## Truth Boundary

- 이 workload는 고정된 synthetic population과 deterministic arrival/return schedule이며 arbitrary external producer traffic을 관찰하지 않는다.
- true skill distribution, game outcome curve와 Elo K-factor는 설명 가능한 reference model이지 production-calibrated MMR이 아니다.
- 영구 이탈/신규 가입 churn, party 재편, 실제 접속률 분포, rating uncertainty, placement match와 season은 모델링하지 않는다.
- planner 내부 candidate 방문을 실시간 stream하지 않는다. operation activity 뒤 완료된 proposal evidence만 표시한다.
- embedded demo는 production scheduler, game server, push delivery, authentication 또는 external allocation server가 아니다.
- raw durable payload와 실제 player identity는 화면이나 tracked fixture에 사용하지 않는다.
- interactive timing은 presentation control이며 match wait와 game duration은 deterministic server clock이 소유한다.

실제 shared server와 game result를 연결하려면 authenticated redacted event stream, external assignment consumer와 versioned result-ingestion contract를 별도로 설계해야 한다.
