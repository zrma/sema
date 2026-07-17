# Public Alpha API

## Scope

`github.com/zrma/sema/alpha`는 Sema의 첫 importable Go API다. immutable snapshot을 받아 side-effect-free `ProposalBatch`를 반환하는 composition surface만 제공한다.

```go
batch, err := alpha.Compose(alpha.Snapshot{
    ID:  "cycle-1",
    Now: now,
    MatchTickets: tickets,
    Policy: alpha.MatchmakingPolicy{
        Version: "policy-v1",
        TeamCount: 2,
        TeamSize: 5,
        MaxLatencyMillis: 200,
    },
})
```

전체 실행 예제는 `examples/compose`에 있다.

```sh
go run ./examples/compose
```

## Contract

- input은 `Snapshot`, `MatchTicket`, optional `BackfillTicket`과 `MatchmakingPolicy`의 independent public copy다.
- output은 `APIVersion: v0alpha1`, proposal/team placement, policy fingerprint, score/search evidence와 unmatched records다.
- input slice와 player data를 internal planner에 직접 alias하지 않고 conversion boundary에서 복사한다.
- 같은 snapshot/policy는 internal deterministic contract와 같은 ordered batch를 만든다.
- invalid input은 public `*alpha.Error`이며 `alpha.ErrorCodeOf`로 stable alpha error code를 읽는다.

`MaxCandidateTickets`, `MaxCandidatesPerProposal`, `MaxSearchNodes`는 각각 discovery input, exact placement comparison과 recursive node budget이다. truncation은 output evidence와 `BudgetExhausted`에 남는다.

## Non-Goals

- coordinator, reservation, assignment와 mutable engine lifecycle을 public으로 노출하지 않는다.
- JSON tag는 example/diagnostic readability를 위한 것이며 production wire protocol을 선언하지 않는다.
- `alpha` package는 v1 source compatibility를 약속하지 않는다.
- authentication, allocation server, transport, storage와 deployment를 포함하지 않는다.

## Verification

- external-surface test: `go test ./alpha`.
- reference consumer: `go run ./examples/compose`.
- full repository gate: `scripts/check.sh`.
