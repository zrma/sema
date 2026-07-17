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

## S20: Offline Policy Simulation

2:2 team, 100-player duo battle royale, backfill과 no-match scenario corpus를 side effect 없이 실행한다. policy와 scenario 입력 순서를 뒤집어도 version/fingerprint와 scenario ID로 정렬된 report는 같아야 한다. conflicting policy는 report 생성 전에 실패하고 각 result는 full batch와 matched/unmatched reason, search budget, score summary를 함께 가진다.

## S21: Executable Evaluation Lab

`cmd/sema-lab`은 team workload matrix의 solo/full-party/mixed-party, 100-player duo/squad, backfill, no-match, latency hard limit와 role/wait objective fixture를 built-in corpus로 제공한다. 선택 workload 입력 순서와 중복은 report에 영향을 주지 않고 반복 실행은 같은 proposal/team placement, ticket/player coverage, unmatched reason과 search evidence를 만든다. text summary, optional detail과 experimental JSON 경로를 실제 command smoke로 검증한다.

## S22: Synthetic Workload And Small-Case Oracle

explicit seed와 weighted party/skill/role/latency/wait parameter는 같은 synthetic queue snapshot을 만든다. report는 player coverage basis points와 oldest unmatched wait를 ticket metric과 분리한다. 최대 12 ticket의 new-match fixture는 exhaustive oracle과 planner 첫 proposal의 objective quality vector를 비교한다. 충분한 candidate budget은 `equivalent`, candidate limit 1의 diagnostic은 더 낮은 skill gap을 가진 `oracle_preferred` evidence를 만들어야 한다.

## S23: Candidate Window And Large Queue

zero `MaxCandidateTickets`는 unbounded result를 유지하고 positive limit은 canonical queue에서 oldest fitting ticket만 exact placement search에 전달한다. 10K solo queue는 256-ticket window로 exact 5:5 proposal, full unmatched accounting과 truncation evidence를 만든다. 10K/100K benchmark gate는 unbounded/window path를 모두 실행한다. candidate-window diagnostic은 oldest quality gap 1000과 oracle gap 0을 함께 기록하고 fuzz target은 input order/immutability, capacity, disjoint와 ticket coverage invariant를 검증한다.

## S24: Public Alpha Consumer And Release Build

external test package가 `alpha.Compose`만 import해 같은 snapshot의 순서를 뒤집어도 동일한 disjoint multi-match batch를 얻고 invalid input을 typed alpha error로 읽는다. `examples/compose`는 direct `internal/` import 없이 실행된다. host `sema-lab` artifact는 explicit version을 출력하고 생성된 `SHA256SUMS` 검증을 통과한다. 실제 tag, release와 remote push는 이 local scenario의 범위 밖이다.

## S25: Durable Restart And Journal Recovery

policy, four-ticket plan과 active reservation을 synced journal에 기록한 뒤 runtime을 다시 열면 같은 policy fingerprint와 reserved ownership이 복구되고 confirm할 수 있다. 같은 snapshot ID plan retry는 server time이 달라도 최초 complete batch를 반환한다. confirmed assignment와 terminal acknowledgment도 restart 뒤 same-ID retry가 동일한 read model을 반환한다. incomplete final tail은 제거하지만 complete corruption, concurrent second writer와 reservation TTL drift는 startup failure다.

## S26: Versioned HTTP Lifecycle And Proposal Authority

`v0alpha1` API로 policy와 four tickets를 ingest하고 plan한 직후 server state를 다시 열어도 response의 proposal ID만으로 reserve할 수 있다. client가 proposal placement 전문을 제출하면 unknown field로 거부한다. confirm 뒤 다시 시작한 server에서 assignment poll과 same-operation terminal acknowledgment retry는 처음 기록한 `acknowledged_at`을 유지한다. malformed/oversized request, path mismatch, missing resource와 implicit non-loopback bind도 typed failure다.

## S27: Redacted Operational Surfaces

concrete ticket/player ID가 포함된 ingestion 뒤 liveness/readiness, metrics, propagated trace와 audit page를 조회한다. metric label과 JSON span은 method와 route pattern만 사용하고 audit은 player count만 남긴다. 세 surface를 직렬화한 결과 어디에도 known resource ID가 없어야 한다. concurrent request observation은 exact counter와 race-free histogram을 유지한다.

## S28: Service Load And Failure Recovery

격리된 single-writer runtime에 실제 HTTP로 2x5 solo workload를 여러 cycle 제출한다. 매 cycle의 모든 ticket은 disjoint proposal, reservation, pending assignment와 completed acknowledgment를 거치며 report는 aggregate operation/audit count와 latency distribution만 노출한다. server를 중지한 뒤 같은 journal을 다시 열면 모든 completed assignment와 audit sequence가 복구되어야 한다. 마지막으로 newline 없는 incomplete record를 append해 disk write interruption을 모사하고 재시작하면 complete prefix만 보존한 채 tail을 제거해야 한다. quick repository gate는 한 cycle을 실행하고 장시간 soak는 같은 command의 cycle 수만 늘리며, 측정값 자체는 아직 제품 SLO가 아니다.

## S29: Hardened Container Restart

pinned multi-stage image는 server, readiness probe와 operational validator를 static binary로 포함하고 numeric non-root identity로 실행한다. in-image validator가 full lifecycle/recovery를 통과해야 한다. service container는 read-only root, bounded tmpfs, zero capabilities와 persistent journal volume으로 시작하고 ready 상태가 된 뒤 graceful stop/start를 거쳐 다시 ready가 되어야 한다. Compose example은 unauthenticated service port를 host loopback에만 publish하고 replica 1을 유지한다.

## S30: Repeated Reference Profile

pinned Linux builder/runtime image를 2 CPU/2 GiB로 제한한다. planner 50v50, 100K candidate window, 1000-ticket engine lifecycle과 1002-event durable replay benchmark를 최소 3회 반복하고 각 metric의 최대 ns/op, B/op와 allocs/op가 versioned budget 안에 있어야 한다. 별도로 10-cycle/20-ticket service lifecycle을 persistent local volume에서 3회 실행해 매번 p95 250ms, single request 1s와 whole run 30s 이하이며 restart/torn-tail recovery가 true인지 확인한다. artifact는 aggregate report만 포함하고 raw benchmark host/CPU output은 폐기한다.

## S31: Release Admission

`v0.*` candidate는 full Go, container, repeated performance/recovery, release build와 publication repository gate를 모두 통과해야 admission된다. major version 1 이상은 같은 검증과 별개로 machine-readable `stable_admitted: true`가 필요하다. 현재 stable API, authenticated remote transport와 external consumer evidence가 없으므로 flag는 false이고 v1 admission은 실제 artifact publish 전에 실패해야 한다.

## S32: Exhaustive Batch Quality Frontier

최대 12 match ticket, 2 backfill ticket과 2 team인 snapshot에서 모든 exact-capacity candidate와 disjoint batch를 열거한다. solo/duo/trio + one-slot backfill fixture는 backfill 1개와 new match 1개로 11명을 모두 선택하고 planner가 `frontier_equivalent`여야 한다. `MaxBatchCandidates=1`인 four-solo 1:1 fixture는 planner 1 proposal/2 player point가 exhaustive 2 proposal/4 player witness에 `frontier_dominated`여야 한다. input 순서를 뒤집어도 frontier와 relation은 같고 ticket이나 backfill target을 재사용하는 supplied batch는 거부한다.

## S33: Default Small-Queue Pareto Planning

candidate limit을 명시하지 않은 12-ticket/2-backfill/2-team 이하 queue는 서로 다른 ticket-set alternative를 expanded candidate graph에 보존한다. weighted party/skill/role/latency/wait와 even-seed backfill을 포함하는 seed 1..128 corpus에서 planner는 모든 exhaustive batch frontier와 `frontier_equivalent`이고 generation/selection truncation이 없어야 한다. direct selector fixture는 utility 200의 gap 0/100 batch보다 utility 160의 gap 10/10 batch를 선택해 rank 합만 높은 dominated batch를 repair한다. explicit one-candidate fixture와 large/single-select path는 기존 bounded evidence와 performance를 유지한다.

## S34: Sustained-Arrival Queue Fairness

skill 0/1000의 오래된 1:1 pair를 queue에 유지하고 매 10초마다 skill 500/500 fresh pair를 추가한다. policy는 30초 전까지 gap 0만 허용하고 그 뒤 gap 1000과 `PrioritizeWait`를 활성화한다. cycle 0/10/20초에는 fresh pair가 선택되지만 30초 cycle에는 오래된 pair가 선택되고 batch evidence는 wait-priority eligible/selected demand 2개와 oldest 30000ms를 기록한다. direct selector는 같은 proposal 수에서 더 높은 fresh rank-sum보다 oldest priority demand를 포함한 batch를 선택한다. explicit truncation이 있으면 bounded service 보장이 아니라 search evidence로 분류한다.

## S35: Roster-Aware Backfill Quality

team size 2의 existing roster는 team A skill total 1000/healer 1, team B skill total 1500/dps 1이고 각 team에 한 slot이 비어 있다. incoming high-dps 1500과 low-healer 1000을 반대 skill team에 배치하면 resulting average gap 0, role penalty 0, max latency 60이 된다. mid 1250 대안보다 이 조합을 planner와 exhaustive frontier가 함께 선택하고 proposal target은 context와 같은 `rosterVersion=7`을 보존한다. higher ticket revision/roster version/context가 ingest된 뒤 이전 proposal reserve는 `StaleSnapshot`이다. empty context는 legacy vacancy-only behavior를 유지한다.

## S36: Indexed Oldest-Prefix Equivalence

party size 1..4, skill 1200..1899, empty/dps/healer role과 latency 20..99ms가 섞인 canonical queue를 party/skill/role/latency partition index로 만든다. 96-ticket queue의 slot shape 1/2/5와 limit 0/1/7/32/128 matrix, 10K queue의 limit 256에서 indexed window는 linear oldest-fitting window의 ticket order와 truncation을 정확히 재현한다. 100K reuse benchmark는 four-shape lookup과 one-time build를 분리해 측정하며 stateless planner가 per-call build를 하지 않는 결정을 검증한다.
