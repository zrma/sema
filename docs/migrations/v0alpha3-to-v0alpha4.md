# Public Go Alpha Migration: v0alpha3 To v0alpha4

## Why The Marker Changed

P25는 wait relaxation이 `PrioritizeWait` 단계에 들어간 demand의 global batch ordering을 바꾼다. v0alpha3는 proposal-level wait가 candidate quality rank에 반영되지만 여러 proposal의 rank 합이 오래된 feasible demand를 포함한 batch를 앞설 수 있었다. v0alpha4는 backfill 수를 먼저 보존한 뒤 oldest eligible priority demand와 selected priority demand 수를 rank utility보다 먼저 비교한다.

같은 snapshot과 policy가 다른 유효 `ProposalBatch`를 만들 수 있고 batch evidence field도 추가되므로 public alpha objective-ordering breaking change로 분류한다.

## Symbol And Field Mapping

기존 type, function, input field와 import path는 유지된다.

| v0alpha3 | v0alpha4 |
|---|---|
| `APIVersion == "v0alpha3"` | `APIVersion == "v0alpha4"` |
| proposal rank sum에서 wait 반영 | oldest eligible priority demand를 batch rank보다 먼저 service |
| proposal evidence의 `WaitPriority` | proposal evidence 유지 + batch priority eligible/selected evidence |
| batch candidate/utility/truncation evidence | 4개 wait-priority aggregate field 추가 |

추가된 `BatchScoreEvidence` field는 `WaitPriorityEligibleDemands`, `WaitPrioritySelectedDemands`, `OldestWaitPriorityMillis`, `OldestSelectedPriorityMillis`다. keyed struct literal은 source change 없이 build되지만 whole-struct comparison, golden JSON과 proposal identity/result는 갱신해야 할 수 있다.

## Consumer Update

literal marker 대신 package marker를 확인하고 priority evidence를 truncation과 함께 읽는다.

```go
batch, err := alpha.Compose(snapshot)
if err != nil {
    return err
}
if batch.APIVersion != alpha.APIVersion {
    return fmt.Errorf("unsupported alpha response")
}
log.Printf("priority=%d/%d oldest=%d selected_oldest=%d truncated=%t",
    batch.Evidence.WaitPrioritySelectedDemands,
    batch.Evidence.WaitPriorityEligibleDemands,
    batch.Evidence.OldestWaitPriorityMillis,
    batch.Evidence.OldestSelectedPriorityMillis,
    batch.BudgetExhausted,
)
```

`BudgetExhausted == true`이면 candidate graph 밖의 feasible demand까지 oldest service가 보장된다고 해석하지 않는다. eligible 수는 hard rejection과 아직 생성되지 않은 placement를 포함하지 않는다.

## Behavior Difference

- 각 match ticket과 backfill ticket은 자신의 wait로 active relaxation step을 계산한다.
- selected backfill 수가 다른 batch 사이의 기존 backfill-first priority는 유지한다.
- 같은 backfill tier에서는 selected oldest priority wait, selected priority demand 수, 기존 Pareto/rank utility 순으로 비교한다.
- priority demand가 없는 snapshot은 v0alpha3 quality ordering을 유지한다.
- window/generation/selection truncation과 best-feasible response 계약은 유지된다.

## Service And Journal Boundary

`/v0alpha1` HTTP response는 같은 네 aggregate를 additive batch evidence로 반환하지만 service marker는 독립적으로 유지한다. `sema-journal-v1`의 plan evidence도 additive field를 기록하고 이전 record의 missing field는 zero value로 replay한다. service와 journal은 public Go marker를 자동으로 공유하지 않는다.

## Rollback

정확한 v0alpha3 selection이 필요하면 P25 이전 module revision을 pin해야 한다. candidate budget을 명시해도 wait-priority ordering은 적용되므로 v0alpha3 rollback 수단이 아니다. v0alpha4 proposal ID와 durable plan decision을 이전 process에서 같은 objective 의미로 재사용할 수 있다고 가정하지 않는다. durable runtime rollback은 repository operations runbook과 backup boundary를 따른다.
