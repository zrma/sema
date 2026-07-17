# P11 Sema Flow TUI Spec

- Status: Complete

## Objective

실제 Sema HTTP lifecycle을 deterministic mixed-party workload로 실행하고 ticket 유입, proposal formation, reservation, confirmation과 departure를 Unicode terminal animation으로 설명한다.

## Deliverables

- 격리 durable runtime과 loopback `v0alpha1` API를 사용하는 serialized flow simulator.
- Bubble Tea v2 full-screen `cmd/sema-tui`.
- solo/duo/trio party glyph, candidate/search evidence와 proposal team placement.
- pause, single-step, speed, Unicode/ASCII와 reduced-motion control.
- terminal-independent deterministic snapshot mode.
- focused lifecycle, renderer width와 compatibility fallback test.

## Acceptance

- simulator가 ticket ingest부터 completed acknowledgment까지 실제 HTTP endpoint를 통과한다.
- plan 한 번이 두 개의 disjoint 5v5 proposal을 만들고 reservation/assignment/departure event가 순서대로 나타난다.
- lifecycle operation 사이에 새 mixed-party ticket이 계속 유입된다.
- Unicode snapshot에 waiting, active와 departed surface가 나타난다.
- ASCII fallback에 box drawing, Braille과 Unicode player glyph가 남지 않는다.
- 120-column snapshot의 모든 line이 terminal width를 넘지 않는다.
- `scripts/check.sh`가 command smoke와 전체 Go/race gate를 통과한다.

## Out Of Scope

- production matchmaking cycle scheduler와 daemon ownership.
- external producer를 포함한 shared queue observer.
- planner candidate-by-candidate live trace.
- backfill/failure injection control과 external allocation animation.
- stable TUI compatibility 또는 release binary distribution.

## Completion Evidence

`go test ./internal/flow ./internal/flowui ./cmd/sema-tui`, Unicode/ASCII snapshot smoke와 `scripts/check.sh`가 통과한다. 동작과 truth boundary는 `docs/sema-flow.md`가 소유한다.
