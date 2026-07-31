# P21 Flow Lifecycle Entry Motion Spec

- Status: Completed

## Objective

Flow TUI의 `MATCH LIFECYCLE`에 새 match가 한 frame에 완성된 block으로 나타나는 단절을 제거한다. `WAITING POOL`에서 같은 marker/color를 가진 party가 선택되어 나가는 흐름과 이어지도록, 새 lifecycle block을 panel 위에서 아래로 순차적으로 펼친다.

## Contract

- `EventPlanCompleted`는 lifecycle 논리 상태를 즉시 갱신하지만 새 block은 첫 presentation frame 전에는 완성된 형태로 렌더링하지 않는다.
- selected party의 hold와 horizontal departure가 끝난 다음 frame부터 한 match block이 header에서 team/evidence 방향으로 네 motion frame에 걸쳐 펼쳐지고, 이미 보이던 lifecycle block은 드러난 row 수만큼 아래로 이동한다.
- 같은 proposal batch의 block은 proposal 순서를 유지하며 한 frame 간격으로 stagger되어 위에서 아래로 채워진다.
- entry 중 reservation/confirmation으로 stage가 바뀌어도 batch 안의 표시 순서와 기존 block의 상대 순서를 재정렬하지 않는다.
- waiting party와 lifecycle block은 기존의 동일한 match marker, color와 active-match visual-slot 수명을 유지한다.
- reservation, confirmation과 playing stage가 entry motion 중 바뀌어도 최신 stage를 표시하며 motion이 해당 전이를 지연하지 않는다.
- reduced-motion과 deterministic snapshot은 중간 frame을 생략하고 최종 lifecycle layout을 즉시 적용한다.

## Acceptance

- plan event 직후 frame에는 새 lifecycle block이 통째로 나타나지 않는다.
- selected party가 queue에서 완전히 빠진 뒤 첫 proposal이 panel 상단에 들어오고 다음 proposal이 stagger되어 뒤따른다.
- 새 row가 드러날 때 기존 playing block의 화면 row가 단조롭게 아래로 이동한다.
- motion 완료 뒤 batch proposal 순서와 기존 lifecycle 순서가 안정적이다.
- reduced-motion, Unicode/color와 ASCII fallback은 기존 terminal bounds를 유지한다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- lifecycle entry frame은 TUI presentation-only read model이다. planner batch, reservation, assignment, game timer와 simulated logical clock의 authority가 아니다.
- 화면 높이 밖의 active match는 기존 summary/viewport 정책을 따르며 entry animation이 lifecycle 보존이나 처리량을 제한하지 않는다.

## Completion Evidence

- frame fixture가 initial no-pop, 첫 block 진입, 다음 block stagger와 기존 playing block의 하향 이동을 순서대로 검증한다.
- reduced-motion fixture가 같은 batch를 첫 render에서 최종 top-to-bottom layout으로 표시한다.
- lifecycle block 높이를 실제 team row 수로 계산해 entry와 panel capacity 판단이 같은 block 경계를 사용한다.
- focused Flow TUI, race, 전체 repository와 publication boundary gate가 통과했다.
