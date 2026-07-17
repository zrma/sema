# Public Go Alpha Migration: v0alpha2 To v0alpha3

## Why The Marker Changed

P24는 `alpha.Compose`의 public type이나 field를 바꾸지 않지만 candidate budget을 생략한 작은 snapshot의 candidate generation과 batch selection 의미를 바꾼다. v0alpha2는 anchor마다 가장 좋은 placement 중심의 bounded candidate graph를 만들고 total rank utility로 batch를 선택했다. v0alpha3는 최대 12 match ticket, 2 backfill, 2 team의 default-budget snapshot에서 서로 다른 ticket set을 더 넓게 보존하고, 같은 coverage tier의 dominated rank-sum batch를 Pareto dominance로 repair한다.

따라서 같은 snapshot과 zero-value candidate budget이 다른 유효 `ProposalBatch`를 만들 수 있어 public alpha objective-ordering breaking change로 분류한다.

## Symbol And Behavior Mapping

기존 type, function, field와 import path는 유지된다.

| v0alpha2 | v0alpha3 |
|---|---|
| `APIVersion == "v0alpha2"` | `APIVersion == "v0alpha3"` |
| zero-value candidate budget은 일반 bounded defaults 사용 | 명시된 small boundary에서 expanded candidate/Pareto path 사용 |
| anchor별 objective-best placement 중심 | 서로 다른 ticket-set alternative를 함께 보존 |
| total rank utility와 canonical key로 batch 선택 | coverage tier 뒤 Pareto dominance, 그 다음 rank utility/canonical key |
| explicit candidate budget | 같은 explicit bounded behavior 유지 |

keyed public struct literal은 source change 없이 build된다. 새 field는 없으며 proposal, proposal ID, evidence와 unmatched 결과는 선택 의미 변경에 따라 달라질 수 있다. positional struct literal은 지원되는 migration path가 아니며 keyed literal을 사용해야 한다.

## Before And After

v0alpha2 behavior를 의도적으로 제한하던 consumer는 candidate budget을 명시한다.

```go
snapshot.Policy.MaxCandidatesPerProposal = 64
snapshot.Policy.MaxBatchCandidates = 256

batch, err := alpha.Compose(snapshot)
if err != nil {
    return err
}
if batch.APIVersion != alpha.APIVersion {
    return fmt.Errorf("unsupported alpha response")
}
```

candidate budget을 0으로 두면 구현 기본값을 선택한다. v0alpha3에서는 small boundary가 expanded path를 사용할 수 있으므로 특정 proposal ID나 team placement를 golden value로 고정한 consumer는 새 결과를 검토해야 한다. semantic marker 검사는 literal 대신 `alpha.APIVersion`을 사용한다.

## Selection Difference

- hard constraint, wait relaxation threshold와 deterministic identity 규칙은 유지된다.
- backfill 수, proposal 수와 matched player 수를 먼저 보존한다.
- 같은 coverage tier에서는 oldest/mean wait를 최대화하고 maximum/mean role penalty, team skill gap과 maximum latency를 최소화하는 Pareto dominance를 적용한다.
- 서로 지배하지 않는 quality trade-off는 v0alpha2의 total rank utility와 canonical key로 결정한다.
- small boundary 밖이거나 candidate limit을 명시한 policy는 v0alpha2의 bounded candidate graph 경로를 유지한다.
- generation/selection node budget이 끝나면 best feasible batch와 truncation evidence를 반환하는 계약은 바뀌지 않는다.

## Service Boundary

이 migration은 importable Go `alpha` package의 marker다. `/v0alpha1` HTTP service와 `sema-journal-v1`은 독립된 experimental surface이며 version marker를 자동으로 공유하지 않는다. service가 같은 planner를 호출하므로 zero-value policy의 계획 결과는 바뀔 수 있지만 wire field와 journal schema 자체는 이 migration에서 바꾸지 않는다.

## Rollback

정확한 v0alpha2 default selection이 필요하면 P24 이전 module revision을 pin해야 한다. explicit candidate limit은 bounded 경로를 보존하지만 모든 v0alpha2 proposal을 재현한다는 compatibility guarantee는 아니다. v0alpha3 proposal ID나 durable plan decision을 이전 process에서 같은 선택 의미로 재사용할 수 있다고 가정하지 않는다. durable runtime rollback은 repository operations runbook과 backup boundary를 따른다.
