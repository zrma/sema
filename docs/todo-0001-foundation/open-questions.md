# P0 Open Questions

## Product

- 첫 reference workload의 team size, party distribution, queue size, latency budget은 무엇인가?
- quality objective에서 skill balance, wait time, latency, role composition의 초기 우선순위는 무엇인가?
- proposal을 여러 개 반환해야 하는 소비자 시나리오가 초기 범위에 포함되는가?

## Consistency

- ticket과 session roster freshness를 어떤 version contract로 판정할 것인가?
- reservation의 authority와 lease owner는 누구인가?
- coordinator 장애 후 assignment 상태의 source of truth는 어디인가?

## Implementation

- Rust와 Go 중 어느 쪽이 representative enumeration/coordination workload에서 더 단순하고 예측 가능한가?
- planner core와 coordinator를 처음부터 별도 process로 나눌 근거가 있는가?
- 초기 persistence가 필요한가, 아니면 deterministic in-memory vertical slice가 먼저인가?

## Decision Gate

위 질문 중 구현 stack과 consistency contract를 바꾸는 항목은 fixture, benchmark, failure scenario 없이 암묵적으로 확정하지 않는다. 답이 없어도 domain schema와 reference fixture 작성까지는 자율적으로 진행할 수 있다.
