# P13 Flow Measurement Baseline Spec

- Status: Complete

## Objective

P12의 deterministic closed-loop simulation을 고정된 simulated duration 동안 실행하고, queue wait, assignment yield, match throughput, queue saturation과 proposal quality를 재현 가능한 aggregate report로 남긴다.

## Metric Contract

- queue wait는 assignment가 confirm된 player entry를 표본으로 하는 player-weighted duration이다.
- assignment yield는 측정 구간에 queue로 유입된 player entry 중 구간 안에 assignment가 confirm된 비율이다. 구간 끝의 대기 player는 right-censored 상태로 분모에 남는다.
- throughput은 측정 구간의 confirmed/completed match 수를 simulated minute당 milli-match 단위로 기록한다.
- queue saturation은 population 대비 queued player 비율이며 event 사이 simulated duration으로 시간 가중한다.
- proposal quality는 confirmed match의 `TeamSkillGap`과 `MaxLatencyMillis` evidence 분포다.
- percentile은 정렬된 표본의 nearest-rank 값을 사용하고 모든 비율은 basis points 또는 정수 fixed-point로 기록한다.

## Acceptance

- 기본 workload는 seed 42, population 1,000명과 P12 timing/policy를 그대로 사용한다.
- `cmd/sema-flow-report`가 duration, seed와 population override를 받아 text와 versioned JSON을 출력한다.
- 같은 configuration은 counts, percentile, occupancy와 final population distribution이 동일한 report를 만든다.
- player-weighted wait, time-weighted queue occupancy와 percentile helper는 작은 synthetic fixture로 경계를 검증한다.
- JSON은 resource identity, raw durable payload, host 정보와 wall-clock timing을 포함하지 않는다.
- command smoke와 focused tests가 repository gate에 포함된다.
- README, Flow 문서, status, roadmap과 handoff가 metric 정의와 truth boundary를 가리킨다.

## Out Of Scope

- production SLA/SLO, 실제 traffic 또는 outcome calibration.
- rating uncertainty, confidence interval, churn과 party 재편.
- multi-seed 통계 추론, hypothesis test와 정책 자동 최적화.
- real-time telemetry endpoint나 shared external queue observer.
- public Go API 또는 stable wire contract.

## Completion Evidence

measurement collector와 CLI focused test, deterministic JSON smoke, Go race gate와 `scripts/check.sh`를 통과한다. report는 synthetic reference evidence이며 제품 SLA가 아님을 `docs/sema-flow.md`에 유지한다.
