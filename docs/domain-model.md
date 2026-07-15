# Domain Model

## Scope

이 문서는 P0에서 Go 코드와 fixture가 공유하는 canonical entity contract를 정의한다. transport schema와 영속 저장소 schema는 아직 고정하지 않는다.

## Identity And Ownership

| Entity | Stable identity | Freshness | Owner | Mutable boundary |
|---|---|---|---|---|
| `Player` | `playerID` | enclosing ticket revision | ticket producer | 새 ticket revision으로만 변경 |
| `MatchTicket` | `ticketID` | monotonically increasing `revision` | ticket producer | coordinator의 active copy를 새 revision으로 교체 |
| `BackfillTicket` | `ticketID` | `revision`과 target `rosterVersion` | session producer | coordinator의 active copy를 새 revision으로 교체 |
| `MatchmakingSnapshot` | `snapshotID` | immutable value | coordinator 또는 adapter | 생성 후 변경하지 않음 |
| `MatchProposal` | `proposalID` | snapshot과 policy version에 종속 | planner | immutable result |
| `ProposalBatch` | `snapshotID` | snapshot과 policy version에 종속 | planner | immutable result |
| `Reservation` | `reservationID` | fixed `expiresAt` | coordinator | 상태만 단방향 전이 |
| `Assignment` | `assignmentID` | confirmed reservation에 종속 | coordinator | 생성 후 변경하지 않음 |

producer는 자신이 소유한 aggregate의 revision을 증가시킨다. coordinator는 revision 값을 새로 만들지 않고 active state와 proposal의 값을 compare-and-swap으로 비교한다.

## Input Entities

### `Player`

- `playerID`: snapshot 안에서 유일한 식별자.
- `skill`: P0의 정수형 대표 skill 값. uncertainty와 rating system은 policy milestone에서 확장한다.
- `role`: game-specific role label. 빈 값은 role을 사용하지 않는 workload를 뜻한다.
- `latencyMillis`: 해당 matchmaking pool 또는 target session에 대한 대표 network latency.

### `MatchTicket`

- `ticketID`, `revision`: identity와 freshness contract.
- `enqueuedAt`: wait time과 deterministic ordering의 기준.
- `players`: 쪼갤 수 없는 한 파티. 한 명이면 solo ticket이다.

`players`는 비어 있을 수 없고 한 ticket의 모든 player는 같은 proposal과 같은 team에 배치된다. 수정은 기존 value의 in-place mutation이 아니라 더 높은 revision의 전체 replacement다.

coordinator는 active ticket과 함께 player ownership을 유지한다. higher revision replacement는 새 player의 중복 검증이 끝난 뒤 old ownership release와 new ownership acquire를 한 mutation으로 적용하며 cancel과 assignment confirm은 ownership을 해제한다.

### `BackfillTicket`

- `ticketID`, `revision`: backfill 수요 자체의 identity와 freshness.
- `sessionID`, `rosterVersion`: 대상 세션과 roster snapshot의 freshness.
- `openSlotsByTeam`: team index별 정확한 빈자리 수.
- `enqueuedAt`: backfill 수요가 생성된 시각.

`openSlotsByTeam`의 모든 값은 음수가 아니며 합계가 1 이상이어야 한다. P0 planner는 현재 roster의 skill과 role을 직접 최적화하지 않고 빈자리 capacity만 사용한다.

### `MatchmakingPolicy`

- `version`: proposal replay와 설명에 남는 stable policy version.
- `teamCount`, `teamSize`: 새 match의 정확한 team shape.
- `maxLatencyMillis`: 절대 network latency hard cap.
- `maxProposals`: 한 batch에서 반환할 proposal 상한. 0은 구현 기본값을 사용한다.
- `maxSearchNodes`: 한 planning cycle의 bounded enumeration 상한. 0은 구현 기본값을 사용한다.
- `maxCandidatesPerProposal`: proposal 하나에서 비교할 exact placement 상한. 0은 구현 기본값을 사용한다.
- `roleRequirements`: team별 hard 또는 soft minimum role count.
- `relaxationSteps`: oldest ticket wait에 따라 skill/role 허용 범위와 wait 우선순위를 바꾸는 ordered schedule.

validated policy는 모든 field를 포함하는 canonical SHA-256 fingerprint를 가진다. role requirement는 unique role name 순으로 canonicalize하고 relaxation step의 순서는 보존한다. fingerprint는 content identity이며 policy 권한이나 signature를 뜻하지 않는다.

same-process policy catalog는 first registration에서 version과 fingerprint를 묶는다. 동일 content 재등록은 idempotent하고 같은 version의 다른 content는 `PolicyConflict`다. catalog와 engine read는 defensive copy이며 process restart 뒤 consumer가 policy를 다시 등록한다.

### `MatchmakingSnapshot`

- `snapshotID`, `now`: replay identity와 wait time 계산 기준.
- `matchTickets`: 새 match와 backfill에 투입 가능한 파티.
- `backfillTickets`: 기존 세션의 빈자리 수요.
- `policy`: 이번 cycle에 적용할 versioned policy.

같은 snapshot 안에서 ticket ID와 player ID는 각각 유일해야 한다. active reservation이 소유한 ticket과 backfill 수요는 새 snapshot에서 제외한다. planner는 입력 slice를 변경하지 않는다.

## Output Entities

### `MatchProposal`

- `proposalID`, `kind`: snapshot, policy fingerprint, canonical placement를 반영한 deterministic identity와 `new_match` 또는 `backfill` 구분.
- `policyVersion`, `policyFingerprint`: caller label과 실제 rule content identity.
- `teams`: team index와 그 team에 배치할 ordered `TicketRef` 목록.
- `tickets`: proposal 전체의 ordered `TicketRef` 목록.
- `backfill`: backfill일 때만 대상 ticket/session/roster version을 기록한다.
- `evidence`: relaxation level, role penalty, team skill gap, oldest/total wait, maximum latency, 비교한 candidate와 search budget.

`TicketRef`는 `ticketID`와 `revision`을 함께 가진다. proposal은 아직 side effect가 없는 제안이며 배치나 좌석을 소유하지 않는다.

### `ProposalBatch`

- `snapshotID`: 입력 snapshot과의 연결.
- `proposals`: deterministic order의 서로 겹치지 않는 proposal 목록.
- `unmatched`: 이번 budget에서 배치하지 못한 active `MatchTicket` 목록.
- `budgetExhausted`: search budget 때문에 best-known 결과로 종료했는지 나타낸다.

각 unmatched 항목은 `hard_constraint`, `insufficient_capacity`, `quality_threshold`, `search_budget`, `proposal_limit` 중 stable 대표 reason을 가진다. 한 batch의 `MatchTicket`은 최대 한 proposal에만 나타난다. 하나의 `BackfillTicket`도 최대 한 backfill proposal에만 나타난다. P0는 같은 ticket 집합에 대한 대안 proposal을 반환하지 않는다.

### `Reservation`

- `reservationID`: caller가 제공하는 opaque idempotency token.
- `proposalID`: 확보한 proposal.
- `ticketRefs`, optional `backfillRef`: reserve 시 검증한 정확한 revision 집합.
- `expiresAt`: renewal 없는 fixed TTL 종료 시각.
- `status`: `active`, `confirmed`, `cancelled`, `expired` 중 하나.

### `Assignment`

- `assignmentID`, `reservationID`, `proposalID`: 확정 요청과 원 proposal의 연결.
- `kind`, `teams`, optional `backfill`: 소비자가 실행할 확정 배치.
- `confirmedAt`: commit 시각.
- `status`: `pending`, `completed`, `cancelled`, `failed` 중 하나.
- optional `acknowledgment`: terminal operation ID, outcome, failure detail, backfill expected/resulting roster version, acknowledged time.

assignment 생성과 동시에 사용한 active ticket은 소비된다. terminal acknowledgment는 외부 allocation/session authority의 적용 결과이며 assignment 취소나 실패는 과거 ticket revision을 자동 복원하지 않는다.

## Hard And Soft Boundary

다음은 항상 hard constraint다.

- party integrity.
- 정확한 team/session capacity.
- absolute network latency cap.
- active ticket/backfill revision과 roster version 일치.
- batch 및 active reservation 사이의 ticket 배타성.

skill balance, soft role composition, wait time, hard cap 안의 상대 latency는 soft objective다. P1은 versioned relaxation step과 replay 가능한 lexicographic evidence를 제공한다. team skill은 player 정수 skill의 team별 평균을 정수 나눗셈으로 계산하며 rating/uncertainty model 자체는 policy 밖의 입력 책임이다.
