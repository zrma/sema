# Policy Simulation

## Purpose

offline simulation은 versioned policy 후보를 immutable demand corpus에 적용해 proposal coverage와 quality evidence를 비교한다. coordinator, reservation, assignment, clock, network와 storage state를 만들거나 변경하지 않는다.

## Input Contract

- 하나 이상의 validated `MatchmakingPolicy` 후보.
- stable scenario ID와 fixed `now`를 가진 하나 이상의 scenario.
- scenario별 match tickets와 optional backfill tickets.

모든 policy는 planning 전에 process-local catalog에 등록된다. 같은 version의 다른 content가 있으면 report를 만들기 전에 `PolicyConflict`다. duplicate scenario ID와 missing identity/time도 `InvalidInput`이다.

## Output Contract

report는 policy version/fingerprint 순, 각 policy 안에서는 scenario ID 순이다. 각 scenario result는 원본 `ProposalBatch`와 다음 summary를 함께 제공한다.

- proposal, matched ticket, unmatched ticket count.
- stable `UnmatchedReason`별 count.
- candidate/node search evidence, truncated proposal count와 budget exhaustion.
- maximum relaxation level, wait-priority proposal count, role penalty, team skill gap, wait와 latency evidence.

같은 policy/corpus는 같은 ordered report를 만들며 policy와 scenario 입력 slice 순서는 결과에 영향을 주지 않는다. summary는 비교 편의용 aggregate이고 proposal `ScoreEvidence`와 unmatched records가 상세 source of truth다.

## Reference Corpus

- 2:2 solo team match.
- 100-player duo battle royale.
- two-slot backfill.
- insufficient-capacity no-match.

file format, remote job, UI와 winner activation은 현재 contract에 포함하지 않는다.

## Verification

- focused: `go test ./internal/simulation`.
- race: `go test -race ./internal/simulation`.
- full repository gate: `scripts/check.sh`.
