# P3 Demand Index Spec

- Status: Planned

## Objective

active ticket ingestion의 player uniqueness 검사를 queue 전체 scan에서 explicit ownership index로 바꾼다. runtime benchmark에서 드러난 queue-size dependent ingestion cost를 줄이되 revision, cancellation, reservation과 assignment semantics는 유지한다.

## Deliverables

- `PlayerID -> TicketID` ownership index를 coordinator state에 추가한다.
- ticket insert, higher-revision replace, cancel과 assignment confirm에서 index를 atomic하게 갱신한다.
- duplicate player rejection, replacement rollback, cancel/re-submit과 confirmed assignment cleanup fixture를 추가한다.
- full engine queue benchmark로 변경 후 path를 재실행한다.

## Acceptance

- 다른 active ticket이 소유한 player는 계속 `InvalidInput`이다.
- 같은 ticket의 higher revision은 제거된 player ownership을 해제하고 새 player ownership을 획득한다.
- validation 실패는 ticket과 index 어느 쪽에도 partial mutation을 남기지 않는다.
- cancellation과 assignment confirm 뒤 과거 player ID를 새 ticket에서 사용할 수 있다.
- focused coordinator/engine test, race detector, runtime benchmark와 전체 repository gate가 통과한다.

## Out Of Scope

- planner candidate index와 queue partition.
- player identity service 또는 cross-process uniqueness.
- persistence, migration과 multi-replica coordination.
- numeric latency SLO.
