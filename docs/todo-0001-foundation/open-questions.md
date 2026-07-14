# P0 Decisions And Open Questions

## Confirmed Product Contract

- 대표 team match는 `2:2`, `3:3`, `5:5`, `10:10`, `16:16`, `20:20`, `50:50`이다.
- battle royale fixture는 총원 100명을 기준으로 2인 party 50개와 4인 party 25개를 각각 다룬다.
- matchmaking 초기에는 skill balance와 role composition 품질을 우선한다. 대기 시간이 늘어나면 wait time을 우선하고, 확장된 후보 안에서는 network latency가 낮은 조합을 우선한다.
- party integrity, capacity, absolute network latency cap은 대기 시간과 관계없이 hard constraint로 유지한다.
- 한 cycle의 결과는 같은 ticket을 중복 사용하지 않는 여러 `MatchProposal`을 담은 `ProposalBatch`다.
- 같은 ticket 집합에 대한 여러 대안 proposal 반환은 P0 범위에 포함하지 않는다.

## Recommended Consistency Defaults

- 각 `MatchTicket`은 stable `ticketID`와 단조 증가하는 `revision`을 가진다. 수정은 새 revision으로 대체하고 취소된 ticket은 active lookup에서 사라진다.
- `BackfillTicket`도 stable `ticketID`와 `revision`을 가지며 대상 `sessionID`와 단조 증가하는 `rosterVersion`을 함께 고정한다. proposal은 사용한 ticket revision과 roster version을 모두 기록한다.
- coordinator는 reserve/commit 시 현재 active revision을 compare-and-swap 방식으로 검증한다. 하나라도 달라지면 mutation 없이 `StaleSnapshot`을 반환한다.
- reservation authority는 reservation을 만들고 해제할 수 있는 유일한 주체를 뜻한다. P0에서는 같은 프로세스의 `Coordinator`만 authority다.
- lease owner는 TTL 동안 confirm/cancel/renew 권한을 가진 주체를 뜻한다. P0에서는 별도 owner model 없이 opaque `reservationID`를 confirm/cancel token으로 사용하고 renew는 지원하지 않는다.
- reservation은 `reservationID`, `proposalID`, 대상 revision, `expiresAt`에 묶이며 confirm 또는 TTL expiry로 종료된다.
- P0의 source of truth는 프로세스 내부 상태다. 재시작하면 미확정 reservation을 모두 폐기하고 producer가 ticket과 session snapshot을 다시 제출한다.
- production durability가 필요해지는 milestone에서는 durable reservation/assignment store를 새 source of truth로 도입한다.

## Confirmed Implementation Baseline

- 구현 언어는 Go다.
- planner core와 coordinator는 package boundary는 분리하되 하나의 deployable process로 시작한다.
- 실제 처리량, 독립 scaling, failure isolation 근거가 생긴 뒤에만 별도 process로 분리한다.
- 첫 vertical slice는 인메모리로 구현하고 production recovery requirement가 구체화된 뒤 영속 저장소를 도입한다.

## Remaining Open Questions

- matchmaking cycle p95 budget, maximum queue wait, absolute network latency cap의 수치는 얼마인가?
- skill을 어떤 값과 uncertainty model로 표현하고 team balance를 어떤 metric으로 비교할 것인가?
- role composition의 필수 조건과 대기 시간에 따라 완화할 수 있는 조건은 각각 무엇인가?
- mixed-party battle royale과 backfill fixture를 P0에 포함할 것인가?

## Decision Gate

남은 수치와 game-specific policy는 reference fixture에서 조정할 수 있다. Go, 단일 프로세스, 인메모리, multi-match `ProposalBatch`, revision/CAS consistency 기본값을 바꾸는 일은 별도 architecture decision으로 다룬다.
