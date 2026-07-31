# P22 Flow Rating Density Scaling Spec

- Status: Completed

## Objective

Tall terminal에서 `AVERAGE QUEUE WAIT`가 analytics panel의 전체 높이를 사용하는 반면 `RATING DENSITY`는 최대 9줄만 렌더링하고 아래 공간을 비워 두는 불균형을 제거한다.

## Contract

- rating density는 exact 1500과 양쪽 25점 단위 `RatingHistogram`을 입력으로 사용하고 visible history의 분포 폭에 따라 대칭 Y축 범위를 선택한다.
- 선택된 범위를 최대 9개 visible band로 묶고 chart 높이가 band 수보다 크면 각 density cell을 가용 row에 비례해 반복해 panel 전체 높이를 채운다.
- bucket label은 반복된 row 중 중앙 row에 한 번만 표시하고 Y axis는 모든 row에서 연속으로 유지한다.
- exact-center bucket이 비어 있을 때 낮은 강조도의 1500 reference axis는 label row 한 줄에만 표시한다.
- chart 높이가 9줄보다 작거나 dynamic range가 넓으면 인접 internal bucket을 visible band로 묶는다.
- 확장은 표시 배율일 뿐 새로운 rating sample을 보간하거나 measurement schema를 바꾸지 않는다.
- Unicode/color와 ASCII fallback은 같은 vertical allocation을 사용한다.

## Acceptance

- 18-row density chart가 정확히 18줄을 반환한다.
- 최대 9개의 dynamic rating label은 각각 한 번만 표시된다.
- 18-row chart에서 exact-center 1500 bucket은 두 density row를 사용한다.
- exact-center bucket이 비어 있으면 reference axis는 확장 높이와 무관하게 한 줄만 사용한다.
- 기본, tall, medium, compact와 ASCII snapshot이 terminal bounds를 유지한다.
- focused, race, full repository와 publication boundary gate를 통과한다.

## Truth Boundary

- 반복된 row는 시각적 cell height이며 추가 population이나 더 정밀한 rating bucket을 뜻하지 않는다.
- density intensity와 1500-centered dynamic histogram은 관찰용 read-model이며 production rating confidence를 뜻하지 않는다.

## Completion Evidence

- tall snapshot에서 rating density의 마지막 dynamic band가 analytics panel 하단까지 배치된다.
- focused fixture가 18줄 사용, label 단일 표시와 1500 bucket의 2-row 확장을 검증한다.
- focused Flow TUI, race, 전체 repository와 publication boundary gate가 통과했다.
