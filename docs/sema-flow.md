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
- 2x5 team, 5초 planning window와 cycle당 proposal upper bound 32개. 한 match 분량부터 partial batch를 반환하며 진행 중인 game 수는 planning을 제한하지 않는다.
- party의 최초 queue 유입 간격 1초, planning 간격 5초.
- simulated game duration 45초, 완료 뒤 최대 복귀 지연 30초, Elo K-factor 32.
- solo/duo/trio party pattern `[2, 1, 1, 1, 3, 2]`의 반복.

빠르게 여러 rating cycle을 보려면 presentation step interval과 최초 유입 간격을 줄인다. `matches-per-cycle`은 고정 batch 크기가 아니라 한 planning cycle의 proposal 상한이다.

```sh
go run ./cmd/sema-tui \
  -interval 50ms \
  -matches-per-cycle 32 \
  -arrival-interval 500ms
```

축소 population을 확인할 수도 있다.

```sh
go run ./cmd/sema-tui -population 100 -game-duration 30s
```

`-arrival-interval`, `-planning-interval`, `-game-duration`, `-max-return-delay`로 각각 최초 유입, planning cadence, 경기 시간과 복귀 대기를 조정한다. `-interval`은 simulated time이 아니라 화면에서 lifecycle step을 진행하는 속도다.

header의 `batch selected/≤limit every interval`은 최근 planning cycle이 반환한 proposal 수, configured upper bound와 snapshot cadence를 나타낸다. 예를 들어 `batch 17/≤32 every 5s`는 32개를 채우지 못해 대기한 것이 아니라 현재 snapshot에서 17개의 admissible disjoint match를 반환했다는 뜻이다.

## Simulation Model

각 player는 stable identity, visible rating, hidden true skill, games와 wins를 가진다. 초기 rating은 모두 1500이지만 true skill은 600부터 2400 사이의 seeded normal-like distribution으로 생성한다. matchmaking ticket에는 visible rating만 기록하므로 planner는 true skill을 알지 못한다.

승패 확률은 양 팀의 평균 true skill 차이에 logistic curve를 적용해 계산하고 seeded draw로 승자를 정한다. 승패가 확정되면 양 팀의 평균 visible rating으로 Elo expectation을 계산하고 각 player에게 같은 team delta를 적용한다. 동일 인원의 5v5에서는 두 팀 delta가 zero-sum이며 rating은 100부터 3000 사이로 제한한다.

confirm된 assignment는 즉시 완료되지 않고 fixed game duration 동안 in-game 상태에 머문다. 시간이 끝나면 completed acknowledgment를 실제 HTTP endpoint에 기록하고, 참가 party는 새 rating과 증가한 revision을 가진 동일 ticket identity로 다시 대기한다. 이 game timer는 synthetic result와 return 시각을 만들 뿐 planning eligibility나 Sema capacity를 제한하지 않는다.

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

`MATCH LIFECYCLE` 패널은 Sema가 confirm한 assignment 이후 frontend가 game을 진행하고 결과를 돌려주는 흐름을 보여주는 관찰 surface다. 진행 중인 match가 많으면 화면 높이에 맞는 최근 항목과 나머지 개수를 요약하며, 패널의 game 수가 새 proposal 생성을 막지는 않는다.

기본 wide layout은 세 행이다. 상단은 `WAITING POOL | MATCH LIFECYCLE`, 중단은 `AVERAGE QUEUE WAIT | RATING DENSITY`, 하단은 `COMPLETED MATCHES | EVENT STREAM`이다.

- average queue wait chart는 assignment confirm 전인 party의 현재 wait를 player 수로 가중한다. Y axis는 wait duration이고 simulated time은 오른쪽으로 흐른다.
- rating density chart는 전체 population을 1500 exact center와 대칭 rating bucket으로 집계한다. 1500 row의 빈 cell은 낮은 강조도의 `─` 기준축이고 실제 density가 있으면 `·`/`░`/`▒`/`▓`/`█` block ramp가 덮는다. `@`는 사용하지 않는다.
- 같은 logical timestamp의 lifecycle event는 trend 한 시점으로 합치고 최근 512 sample만 유지한다.
- 기본 Unicode mode는 wait chart의 `●`/`░`와 event stream의 `→`/`◉`/`◆`/`▶`/`✓` marker를 사용하며 ASCII symbol은 `-ascii` fallback에서만 사용한다.
- proposal에 선택된 party row는 즉시 사라지지 않는다. 같은 match에 속한 party는 동일한 color와 numbered marker로 잠시 고정된 뒤 오른쪽으로 함께 이동하고, 완전히 빠져나간 뒤 남은 queue row가 frame마다 한 줄씩 위로 접힌다. 같은 marker와 color는 `MATCH LIFECYCLE`의 header, team과 evidence 전체에 유지되고 lifecycle stage는 glyph와 text로 구분한다. active match가 끝나기 전에는 visual slot을 재사용하지 않는다.
- queue departure와 vertical compaction은 presentation motion이며 ticket reservation, confirm 시각이나 simulated clock을 바꾸지 않는다. reduced-motion과 snapshot은 중간 frame을 생략하고 최종 queue layout을 즉시 적용한다.
- 72 columns 미만 또는 30 rows 미만에서는 lifecycle/recent summary를 우선하는 compact view로 축약한다.
- trend는 관찰용 read model이며 planning, measurement나 rating authority가 아니다.

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
  -game-duration 20s -max-return-delay 10s \
  -steps 80 -width 100 -height 32
```

snapshot은 color와 staged motion을 끄고 terminal identity가 없는 plain text를 출력한다. 기본 frame에는 waiting population, in-game match, completed result와 벌어진 rating range가 함께 나타난다.

## Measurement

같은 closed loop의 wait, throughput, queue saturation과 proposal quality를 고정된 simulated duration으로 비교하려면 headless report를 사용한다.

```sh
go run ./cmd/sema-flow-report
go run ./cmd/sema-flow-report -format json -duration 10m
```

due arrival은 이미 예약된 server-clock 시각의 event이므로 처리 frame 자체가 simulated queue time을 추가하지 않는다. scheduler는 다음 arrival, lifecycle stage, game completion과 planning eligibility 중 가장 이른 logical timestamp로 전진하며 같은 timestamp의 여러 event는 여러 TUI frame으로 보여도 clock을 더 전진시키지 않는다. `ready`는 scheduled timestamp가 지난 ingress backlog이고 `cooldown`은 아직 복귀 시각이 오지 않은 player다. metric 정의와 기본 30분 aggregate는 `docs/sema-flow-measurement.md`, multi-seed profile 비교는 `docs/sema-flow-capacity-matrix.md`가 소유한다.

## Truth Boundary

- 이 workload는 고정된 synthetic population과 deterministic arrival/return schedule이며 arbitrary external producer traffic을 관찰하지 않는다.
- true skill distribution, game outcome curve와 Elo K-factor는 설명 가능한 reference model이지 production-calibrated MMR이 아니다.
- 영구 이탈/신규 가입 churn, party 재편, 실제 접속률 분포, rating uncertainty, placement match와 season은 모델링하지 않는다.
- planner 내부 candidate 방문을 실시간 stream하지 않는다. operation activity 뒤 완료된 proposal evidence만 표시한다.
- embedded demo는 production scheduler, game server, push delivery, authentication 또는 external allocation server가 아니다.
- Sema의 책임은 ticket ingestion부터 assignment confirm까지다. confirm 이후 game 실행, 결과 생성과 result submission은 frontend/game runtime 책임이며 Flow는 시각화를 위해 이를 synthetic하게 모사한다.
- raw durable payload와 실제 player identity는 화면이나 tracked fixture에 사용하지 않는다.
- interactive timing은 presentation control이며 match wait와 game duration은 deterministic server clock이 소유한다.
- headless aggregate는 synthetic Flow workload evidence이며 production throughput, queue wait SLA나 traffic calibration이 아니다.

실제 shared server와 game result를 연결하려면 authenticated redacted event stream, external assignment consumer와 versioned result-ingestion contract를 별도로 설계해야 한다.
