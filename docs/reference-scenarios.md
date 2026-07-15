# Reference Scenarios

## Fixture Conventions

- 모든 fixture는 고정 `snapshotID`, `now`, ticket ID, revision, policy version을 사용한다.
- correctness fixture의 `maxLatencyMillis`는 `200`, 기본 search budget은 `100000` nodes다. 이는 제품 SLO가 아니라 hard constraint와 bounded search를 실행하기 위한 test value다.
- team match는 두 team과 format별 `teamSize`를 사용한다.
- 같은 fixture를 반복 실행해 serialized proposal order가 같은지 비교한다.

## S1: Disjoint Multi-Match

`2:2` policy와 여덟 solo ticket을 입력한다.

- 정확히 두 개의 `new_match` proposal을 반환한다.
- 각 proposal은 두 team에 두 명씩 배치한다.
- 여덟 ticket은 한 번씩만 나타나고 unmatched는 비어 있다.
- proposal ID와 team/ticket ordering은 반복 실행에서 같다.

## S2: Party Preservation

`3:3` policy에 2인 party 두 개와 solo 두 개를 입력한다.

- 두 team을 정확히 세 명으로 채운다.
- 각 2인 party는 한 team 안에 그대로 남는다.
- party를 쪼개야만 capacity를 채울 수 있는 입력은 no-match다.

## S3: Team Workload Matrix

`2:2`, `3:3`, `5:5`, `10:10`, `16:16`, `20:20`, `50:50` 각각에서 다음을 실행한다.

- `all-solo`: 정원 수만큼 1인 ticket.
- `full-party`: 각 team 정원과 같은 크기의 party 두 개.
- `mixed-party`: 각 team을 정확히 채우는 party와 solo 조합.

모든 variant는 정확한 capacity, party integrity, deterministic ordering을 검증한다.

## S4: Battle Royale Party Envelope

한 team, 정원 100명인 policy를 사용한다.

- duo: 2인 party 50개.
- squad: 4인 party 25개.

각 fixture는 하나의 100인 proposal, party 보존, empty unmatched를 검증한다. mixed-party battle royale은 P0 correctness 범위 밖이다.

## S5: Backfill Before New Match

두 team에 각각 한 자리씩 빈 `BackfillTicket` 하나와 네 solo `MatchTicket`을 입력한다.

- 첫 proposal은 두 solo를 사용하는 `backfill`이다.
- 남은 두 solo는 새 `2:2` match를 만들 수 없으므로 unmatched다.
- proposal은 backfill ticket revision, session ID, roster version을 보존한다.

## S6: No-Match Hard Constraints

- 빈자리보다 큰 party만 있으면 backfill proposal을 만들지 않는다.
- player latency가 `maxLatencyMillis`를 넘는 ticket은 어떤 proposal에도 포함하지 않는다.
- 정확한 team capacity를 채울 수 없으면 부분 new match를 만들지 않는다.

## S7: Stale Revision

revision 1의 ticket으로 만든 proposal 이후 coordinator active ticket을 revision 2로 교체한다.

- revision 1 proposal의 reserve는 `StaleSnapshot`이다.
- 어떤 ticket도 부분 reservation 상태가 되지 않는다.
- 새 snapshot은 revision 2만 포함한다.

backfill variant에서는 `BackfillTicket.revision` 또는 `rosterVersion` 중 하나만 바뀌어도 같은 결과를 기대한다.

## S8: Reservation Conflict And Retry

같은 ticket을 포함한 두 proposal을 서로 다른 batch에서 준비한다.

- 첫 reservation은 성공한다.
- 두 번째 reservation은 `ReservationConflict`이며 부분 resource를 잡지 않는다.
- 첫 reservation을 cancel하거나 expire한 뒤 새 reservation ID로 다시 시도하면 성공한다.

## S9: Idempotent Confirm

같은 `reservationID`, proposal, `assignmentID`로 reserve와 confirm을 각각 반복한다.

- 반복 reserve는 동일 reservation을 반환하고 TTL을 늘리지 않는다.
- 반복 confirm은 동일 assignment를 반환한다.
- 같은 ID에 다른 proposal 또는 assignment를 연결하면 `IdempotencyConflict`다.

## Performance Evidence

benchmark는 workload matrix의 planning 경로를 실행하고 allocations와 elapsed time을 기록한다. 초기 P0에서는 pass/fail SLO를 두지 않는다. 결과가 쌓이면 `maxSearchNodes`, queue size, cycle p95 budget을 함께 고정한다.

## S10: Quality-First Candidate Ranking

같은 capacity를 채우는 여러 후보 중 짧게 기다린 fixture는 soft role penalty가 낮은 후보, team skill gap이 낮은 후보, wait가 긴 후보, latency가 낮은 후보 순으로 선택한다. 같은 입력 순서를 뒤집어도 canonical proposal은 같다.

## S11: Wait-Based Relaxation

short step에서 role/skill threshold를 넘는 exact placement는 `quality_threshold` unmatched로 남는다. 같은 ticket이 relaxed step threshold보다 오래 기다리면 허용 범위 안에서 match가 생기고 evidence에 relaxation level과 wait-first ordering이 남는다. hard role, party, capacity, absolute latency는 완화되지 않는다.

## S12: Search And Unmatched Evidence

candidate 또는 node cap에 도달하면 best-known proposal을 보존하고 `searchTruncated`와 batch budget outcome을 기록한다. match가 생기지 않거나 proposal 상한 뒤 ticket이 남으면 모든 unmatched ticket이 stable 대표 reason을 가진다.

## Queue Benchmark

5:5 solo queue의 100, 500, 1000 ticket에서 proposal 하나의 64-candidate bounded comparison을 실행한다. P1 gate는 benchmark가 실행 가능하고 결정적인지만 확인하며 machine-specific timing을 제품 SLO로 기록하지 않는다.

## Runtime Application Benchmark

planner-only benchmark와 별도로 same-process engine에서 새 state 생성, ticket ingestion, plan, reserve와 confirm을 실행한다. 2:2 solo, 50:50 solo, 100-player duo battle royale과 100/500/1000 ticket queue를 포함하며 proposal, unmatched reason, search budget과 pending assignment metric을 보고한다.

## S13: Assignment Acknowledgment

confirmed new-match assignment는 `pending`으로 시작한다. 같은 operation ID와 completed payload의 반복은 첫 acknowledged time을 포함한 동일 read model을 반환한다. 같은 ID의 다른 payload는 `IdempotencyConflict`, 다른 operation의 terminal 전이는 `InvalidTransition`이다.

## S14: Backfill Roster CAS Handoff

backfill completion은 assignment의 session ID와 expected roster version을 그대로 사용하고 더 높은 resulting version을 요구한다. non-advancing version은 assignment를 pending으로 남긴다. 외부 authority가 newer roster를 관측한 fixture는 `StaleSnapshot` failed acknowledgment로 terminal state를 남긴다.

## S15: Cancellation And Concurrency

assignment cancellation은 consumed ticket을 자동 복원하지 않는다. complete와 cancel이 동시에 도착하면 정확히 하나만 성공하고 다른 요청은 typed terminal transition failure다.

## S16: Process Restart And Producer Replay

active reservation이 있는 engine 대신 새 engine을 만들면 demand와 reservation은 비어 있다. producer가 같은 active tickets를 재제출하고 같은 snapshot identity, time, policy를 사용하면 restart 전과 같은 proposal이 만들어진다. 이전 reservation ID는 새 process에서 다시 사용할 수 있으며 confirmed assignment read model도 복구되지 않는다.

## S17: Engine Failure Boundaries

fixed TTL에 도달한 reservation의 confirm은 `ReservationExpired`이고 proposal의 모든 ticket은 다음 cycle에 함께 나타난다. 같은 pending assignment에 complete와 cancel acknowledgment가 동시에 도착하면 하나만 terminal 상태를 기록하고 다른 요청은 `InvalidTransition`이다.

## S18: Policy Content Identity

같은 snapshot, policy content와 placement를 반복하면 같은 fingerprint와 proposal ID가 만들어진다. role requirement 입력 순서만 바꾼 policy도 같은 fingerprint다. 같은 version에서 latency cap 등 rule content를 바꾸면 placement가 같아도 fingerprint와 proposal ID가 달라지고, 같은 reservation ID에 두 proposal을 사용하면 `IdempotencyConflict`다.

## S19: Process-Local Policy Catalog

first registration과 same-content retry는 같은 fingerprint/read model을 반환한다. 같은 version의 changed rule은 `PolicyConflict`이고 기존 entry가 유지된다. concurrent first registration은 정확히 한 content만 선택한다. engine은 registered version으로만 plan하며 새 process에는 policy entry가 없다.
