# P27 Indexed Candidate Discovery Spec

- Status: Completed

## Objective

large queue에서 반복 candidate-window 조회를 재사용할 partition index seam을 구현한다. index가 quality preference나 fairness contract를 암묵적으로 바꾸지 않도록 linear oldest-fitting prefix와 정확히 같은 output/evidence를 만들고, build와 lookup cost를 분리해 stateful ownership 결정을 남긴다.

## Partition And Selection

canonical queue는 party size, party average skill 100-point band, role multiset profile과 maximum latency 25ms band로 partition한다. 이 band는 candidate inclusion policy가 아니라 lookup structure다.

- party-size 종류가 16개 이하이면 각 fitting threshold의 canonical queue position을 cache한다.
- 더 넓은 party envelope은 fitting partition head를 enqueue time, ticket ID, revision으로 heap merge한다.
- `limit <= 0`은 original unbounded queue를 반환한다.
- positive limit은 linear `SelectWindow`와 같은 ticket order 및 `Truncated`를 반환한다.
- profile collision은 partition을 합칠 수 있지만 canonical merge 결과에는 영향을 주지 않는다.

## Lifecycle Boundary

100K queue에서 reusable indexed lookup과 repeated shape scan은 같은 microsecond-scale이며 shape와 측정 noise에 따라 우위가 바뀐다. one-time build cost와 memory는 한두 lookup 차이보다 크므로 current `Plan`은 immutable snapshot마다 index를 rebuild하지 않는다. index를 queue mutation과 함께 incremental하게 유지하고 여러 plan에서 재사용하는 책임은 persistence/API productization의 stateful demand repository가 소유한다.

이 boundary는 unfinished matcher preference가 아니다. index의 key, exact selection semantics, fallback과 measurement는 완료되었고 deployment lifetime만 state owner와 함께 연결한다.

## Acceptance

- 96-ticket mixed queue의 3 slot shape x 5 limit matrix가 linear window와 `DeepEqual`이다.
- 10K mixed queue의 3 shape/limit 256 output과 truncation이 exact-equivalent다.
- input permutation이 아니라 canonical queue order가 source of truth임을 문서화한다.
- 100K repeated four-shape lookup과 one-time build benchmark를 분리한다.
- existing 10K/100K planner, P25 fairness와 P24 frontier gate가 그대로 통과한다.
- full/race/publication gate를 통과한다.

## Truth Boundary

- 100/25 band는 product skill 또는 latency bucket이 아니며 public policy/API에 노출하지 않는다.
- region/geography partition은 실제 consumer contract 전까지 추가하지 않는다.
- benchmark elapsed time은 machine-specific observation이며 product SLO가 아니다.
- incremental mutation, persistence와 multi-writer consistency는 다음 stateful service milestone의 책임이다.

## Completion Evidence

- small/10K exact-equivalence tests가 party size, skill, role, latency가 섞인 deterministic queue를 검증한다.
- 100K benchmark가 linear/indexed lookup에서 같은 output allocation을 기록하고 build cost를 별도 항목으로 노출한다.
- stateless planner reference benchmark는 index build allocation을 포함하지 않는다.
