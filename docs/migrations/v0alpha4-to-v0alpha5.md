# Public Go Alpha Migration: v0alpha4 To v0alpha5

## Why The Marker Changed

P26은 `BackfillTicket`에 optional existing-roster aggregate를 추가하고, aggregate가 있을 때 vacancy를 채운 뒤 resulting skill/role/latency를 평가한다. v0alpha4 backfill은 incoming party의 wait/latency와 vacancy shape만 비교했지만 v0alpha5는 같은 snapshot에서 다른 placement를 선택하거나 quality threshold를 적용할 수 있다.

public input shape와 objective ordering이 함께 바뀌므로 breaking alpha marker를 올린다.

## Symbol And Field Mapping

| v0alpha4 | v0alpha5 |
|---|---|
| `APIVersion == "v0alpha4"` | `APIVersion == "v0alpha5"` |
| `BackfillTicket.OpenSlotsByTeam` | 기존 field 유지 |
| existing roster context 없음 | optional `BackfillTicket.ExistingTeams` |
| vacancy-only skill/role evidence | context가 있으면 resulting roster evidence |
| raw `"batch_objective"` reason 가능 | `alpha.UnmatchedBatchObjective` named constant 제공 |

새 public type은 `RosterTeamSummary`와 `RoleCount`다. keyed struct literal은 기존 source를 그대로 build하고 empty `ExistingTeams`는 v0alpha4 vacancy-only behavior를 유지한다. positional struct literal과 whole-struct golden comparison은 지원되는 migration path가 아니다.

## Consumer Update

roster context를 제공하는 consumer는 active `rosterVersion`과 같은 snapshot에서 aggregate를 만든다.

```go
backfill.ExistingTeams = []alpha.RosterTeamSummary{
    {
        PlayerCount: 4,
        SkillTotal:  6000,
        RoleCounts: []alpha.RoleCount{
            {Role: "healer", Count: 1},
        },
        MaxLatencyMillis: 60,
    },
    // one entry per team
}

batch, err := alpha.Compose(snapshot)
if err != nil {
    return err
}
if batch.APIVersion != alpha.APIVersion {
    return fmt.Errorf("unsupported alpha response")
}
```

각 team에서 `PlayerCount + OpenSlotsByTeam`은 policy team size와 같아야 한다. context 또는 실제 roster가 바뀌면 producer는 ticket revision을 전진시키고 roster 변화에는 `RosterVersion`도 전진시킨다.

## Behavior Difference

- context가 있으면 incoming placement를 existing skill total/player count에 더해 resulting average gap을 계산한다.
- existing role count와 incoming role을 합쳐 hard/soft role requirement를 평가한다.
- existing maximum latency는 proposal evidence에 포함되고 incoming latency absolute cap은 계속 hard constraint다.
- context가 없으면 v0alpha4 vacancy-only evaluation을 유지한다.
- backfill-first, queue fairness, candidate budget과 truncation semantics는 바뀌지 않는다.

## Service And Journal Boundary

`/v0alpha1` backfill DTO에도 `existing_teams`가 additive field로 추가되지만 service marker는 독립적으로 유지한다. `sema-journal-v1`은 backfill ticket 안의 aggregate를 기록하고 이전 record의 missing field를 empty legacy context로 replay한다. proposal target은 aggregate 전문 대신 ticket revision과 `rosterVersion`을 보존한다.

## Rollback

context가 없는 input은 v0alpha4 behavior와 호환된다. context를 사용해 만든 v0alpha5 plan을 이전 process에서 같은 objective 의미로 재사용할 수는 없다. 정확한 rollback은 P26 이전 module revision과 context-free producer payload를 함께 사용해야 하며 durable runtime rollback은 operations runbook과 backup boundary를 따른다.
