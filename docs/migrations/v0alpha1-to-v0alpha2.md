# Public Go Alpha Migration: v0alpha1 To v0alpha2

## Why The Marker Changed

P18은 `alpha.Compose`의 출력 struct에 evidence를 추가할 뿐 아니라 proposal을 선택하는 의미를 바꾼다. v0alpha1은 best individual proposal을 선택하고 사용 ticket을 제거하는 과정을 반복했다. v0alpha2는 diverse admissible candidate graph를 먼저 만든 뒤 서로 겹치지 않는 proposal 집합의 total rank utility를 bounded weighted set-packing으로 최적화한다.

따라서 같은 snapshot과 기존 policy가 다른 유효 `ProposalBatch`를 만들 수 있어 public alpha objective-ordering breaking change로 분류한다.

## Symbol And Field Mapping

기존 type/function 이름과 import path는 유지된다.

| v0alpha1 | v0alpha2 |
|---|---|
| `APIVersion == "v0alpha1"` | `APIVersion == "v0alpha2"` |
| `MatchmakingPolicy.MaxProposals` exact fill처럼 보일 수 있음 | 항상 반환 상한 `0..MaxProposals` |
| candidate-generation limits만 제공 | `MaxBatchCandidates`, `MaxBatchSearchNodes` 추가 |
| proposal search evidence | `ScoreEvidence.SelectionUtility` 추가 |
| batch에 count/unmatched/budget flag | `ProposalBatch.Evidence`에 global selection evidence 추가 |
| unmatched reason 5종 | `batch_objective` 추가 |

기존 keyed struct literal은 source change 없이 build된다. 새 policy field의 zero value는 구현 기본값을 사용한다. positional struct literal은 지원되는 migration path가 아니며 keyed literal로 바꿔야 한다.

## Before And After

v0alpha1 consumer가 literal marker와 proposal count만 확인했다면:

```go
batch, err := alpha.Compose(snapshot)
if err != nil {
    return err
}
if batch.APIVersion != "v0alpha1" {
    return fmt.Errorf("unsupported alpha response")
}
```

v0alpha2에서는 package marker를 기준으로 확인하고 batch evidence를 함께 읽는다. limit을 생략하면 구현 기본값을 사용한다.

```go
snapshot.Policy.MaxBatchCandidates = 256
snapshot.Policy.MaxBatchSearchNodes = 100_000

batch, err := alpha.Compose(snapshot)
if err != nil {
    return err
}
if batch.APIVersion != alpha.APIVersion {
    return fmt.Errorf("unsupported alpha response")
}
log.Printf("selected=%d candidates=%d utility=%d truncated=%t",
    batch.Evidence.SelectedProposals,
    batch.Evidence.CandidateProposals,
    batch.Evidence.TotalUtility,
    batch.BudgetExhausted,
)
```

## Behavior Difference

- 각 candidate는 기존 hard constraint와 wait-relaxed quality threshold를 먼저 통과한다.
- backfill proposal 수를 먼저 최대화하고 그 안에서 total rank utility를 최대화한다.
- rank utility는 admissible match의 admission baseline과 quality rank를 더하며 해당 snapshot의 bounded candidate graph 안에서만 의미가 있다.
- candidate generation 또는 selection budget이 끝나면 오류 대신 best feasible batch와 truncation evidence를 반환한다.
- 새 batch limit도 canonical policy fingerprint에 포함되므로 policy content와 proposal identity가 달라진다.

## Service Boundary

이 migration은 importable Go `alpha` package의 marker다. `/v0alpha1` HTTP service는 별도 experimental wire surface이며 route marker를 자동으로 공유하지 않는다. P18 service DTO의 policy/evidence field는 additive지만 planning semantics는 같은 planner를 사용한다.

## Rollback

v0alpha1 selection behavior가 필요하면 P18 이전 module revision을 pin해야 한다. v0alpha2 journal/policy fingerprint를 v0alpha1 process에서 같은 decision identity로 재사용할 수 있다고 가정하지 않는다. durable runtime rollback은 repository operations runbook과 backup boundary를 따른다.
