# P17 Flow Trend Panels Spec

- Status: Completed

## Objective

Flow TUI가 현재 상태와 최근 event만 보여주는 데서 끝나지 않고, player-weighted queue wait와 전체 population rating 분포가 simulated time에 따라 어떻게 변하는지 한 화면에서 비교 가능하게 한다.

## Layout Contract

- 상단은 기존 `WAITING POOL | MATCH LIFECYCLE`을 유지한다.
- 중단은 `AVERAGE QUEUE WAIT | RATING DENSITY`를 같은 높이로 배치한다.
- 하단은 기존 full-width 두 행을 합쳐 `COMPLETED MATCHES | EVENT STREAM`으로 배치한다.
- 기본 120x38과 tall terminal은 전체 높이를 정확히 사용하고, 72 columns 미만 또는 30 rows 미만은 compact view로 축약한다.

## Trend Contract

- average queue wait는 assignment confirm 전 ticket의 현재 wait를 party player 수로 가중한 값이다.
- 같은 10초 simulated logical-time bucket의 여러 lifecycle event는 마지막 read model로 교체해 한 시간대가 여러 열을 차지하지 않게 한다.
- trend history는 최근 512 fixed-time bucket으로 bounded된다.
- trend column은 10초 simulated logical-time bucket으로 고정한다. 신규 bucket은 기존 column을 값 그대로 왼쪽으로 이동시키고 같은 bucket의 event만 최신 column을 교체하며, retained history를 화면 폭에 맞춰 재투영하지 않는다.
- wait chart는 왼쪽 Y axis에 동적인 duration scale, 오른쪽 방향에 simulated time을 둔다.
- rating density는 전체 population을 exact 1500과 양쪽 25점 internal bucket으로 집계하고 visible history의 실제 분포 폭에 맞춰 1500 중심 Y축을 대칭 확장/축소한다.
- density cell은 population share가 클수록 `·`/`░`/`▒`/`▓`/`█` 순서로 차오르고 color도 강해지며 `@`는 사용하지 않는다. 1500 center row의 빈 cell은 낮은 강조도의 `─` 기준축이고 실제 density가 있으면 같은 block ramp가 그 위를 덮는다.
- 기본 Unicode mode는 wait chart와 event stream marker에도 Unicode glyph를 사용하고 `*`/`:`/`o`/`#` 같은 visual marker는 ASCII fallback에만 둔다.
- 기존 measurement histogram과 report schema는 변경하지 않는다.

## Queue Motion Follow-up

- proposal에 선택된 party row는 다음 render에서 즉시 제거하지 않고 match별 color/marker를 공유하는 selected hold 상태를 거친다.
- 같은 proposal의 party는 동일 frame에 오른쪽으로 이동하며 `MATCH LIFECYCLE` block 전체의 marker/color와 시각적으로 연결된다.
- active match는 완료될 때까지 고유한 visual slot을 점유하고, 동시에 존재하는 다른 lifecycle match와 marker/color를 공유하지 않는다.
- Unicode visual slot 1–20은 `①`–`⑳`, 이후 slot은 일반 숫자를 사용하되 waiting/lifecycle 모두 고정 3-cell marker column으로 정렬한다.
- departure가 끝난 row만 display queue에서 빠지고, 아래 row는 target position까지 frame당 한 줄씩 올라온다.
- reduced-motion과 deterministic snapshot은 selected hold, horizontal travel과 intermediate compaction frame을 생략한다.
- motion state는 TUI read model이며 planner proposal, reservation, confirmation이나 logical clock을 지연하지 않는다.

## Acceptance

- Unicode/color, no-color, ASCII와 reduced-motion snapshot이 width/height를 넘지 않는다.
- 기본 wide snapshot에서 analytics 두 panel과 recent 두 panel의 title이 각각 같은 행에 있다.
- 1500 중심에서 rating이 상하로 갈라지는 density history가 deterministic하게 나타난다.
- current wait는 player-weighted이며 confirmed participant는 분모에서 제외된다.
- `MATCH LIFECYCLE` panel과 active-game 요약은 그대로 유지한다.
- 같은 proposal의 selected party가 marker를 공유하고 hold, horizontal departure와 incremental vertical compaction 순서로 렌더링된다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- chart는 TUI read model이며 planner, coordinator, measurement와 game runtime의 authority가 아니다.
- wait scale과 density color는 현재 bounded history의 시각화이며 product SLA나 calibrated rating confidence가 아니다.
- compact terminal에서는 trend panel을 생략할 수 있지만 lifecycle과 recent event 요약은 유지한다.

## Completion Evidence

- 기본 120x38 snapshot에서 analytics와 recent panel이 각각 같은 행에 있고 `MATCH LIFECYCLE`은 active game 18개를 요약하며 유지된다.
- player-weighted wait fixture가 solo 10초와 trio 4초를 5.5초로 집계하고 confirmed participant를 제외한다.
- 10-player match fixture가 1500에서 1484/1516으로 갈라진 population을 fine-grained histogram의 대칭 bucket에 5명씩 기록하고, 1300/1700 fixture가 Y축을 1300–1700으로 확장한다.
- 같은 10초 bucket replacement와 value-preserving horizontal scroll, Unicode/color, no-color, ASCII, medium 80x38, tall 140x56와 compact 80x24 snapshot이 정확한 terminal bounds를 통과한다.
- frame fixture에서 같은 proposal의 두 party가 `①` marker를 공유하고 hold 뒤 함께 퇴장하며, 뒤 row가 두 frame에 걸쳐 `2 → 1 → 0`으로 올라온다. reduced-motion fixture는 즉시 final row를 적용한다.
- focused race, full repository와 publication boundary gate를 통과했다.
