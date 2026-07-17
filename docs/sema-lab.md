# Sema Lab

## Purpose

`sema-lab`은 Sema의 deterministic planner를 고정된 reference workload에 실행하고 실제 proposal과 decision evidence를 확인하는 로컬 평가 도구다. network, database, coordinator state를 만들지 않으며 `internal/simulation` 위에서 side effect 없이 동작한다.

이 실행 파일은 안정적인 SDK나 production service가 아니다. built-in workload와 `v0alpha2` JSON은 P6 평가 계약을 발견하기 위한 실험 표면이며 호환성을 아직 약속하지 않는다.

## Commands

```sh
go run ./cmd/sema-lab -list
go run ./cmd/sema-lab team-2v2-mixed backfill-2v2-two-slots
go run ./cmd/sema-lab -details quality-wait-relaxed-2v2
go run ./cmd/sema-lab -format json battle-royale-duo
```

workload 인자를 생략하면 전체 built-in corpus를 scenario ID 순서로 실행한다. 같은 workload 집합은 입력 순서나 중복 여부와 관계없이 같은 ordered report를 만든다.

## Built-in Corpus

- `2:2`, `3:3`, `5:5`, `10:10`, `16:16`, `20:20`, `50:50`의 solo, full-party, mixed-party team match.
- 100-player duo와 squad battle royale.
- two-slot backfill과 insufficient-capacity no-match.
- absolute latency hard-limit rejection.
- role-quality selection과 wait-based role relaxation.
- seeded synthetic 5:5 queue와 intentional bounded-quality-gap diagnostic.

모든 fixture는 fixed snapshot time, stable entity identity, revision과 policy content를 사용한다. workload의 latency cap과 search budget은 correctness와 비교 실행을 위한 reference value이며 production SLO가 아니다.

## Report Contract

기본 text report는 workload별로 다음 aggregate를 출력한다.

- demand match ticket, backfill ticket과 player count.
- proposal, matched/unmatched ticket과 player count.
- stable unmatched reason distribution과 budget exhaustion.
- candidate, search node, truncation, relaxation, role, skill, wait와 latency evidence.
- player coverage basis points, oldest matched/unmatched wait와 eligible small-case oracle relation.

`-details`는 proposal ID, kind와 team별 ticket placement를 추가한다. JSON report는 같은 aggregate와 모든 proposal placement/evidence를 `schema_version: v0alpha2` envelope에 담는다. oracle은 12 ticket 이하의 new-match workload에만 포함되며 global batch optimum을 의미하지 않는다.

canonical report에는 실행 시간이나 allocation처럼 머신에 따라 달라지는 값을 넣지 않는다. elapsed time과 allocation은 Go benchmark가 측정하며 P6에서 비교 기준과 regression budget을 정한다.

## Verification

- focused: `go test ./internal/lab ./cmd/sema-lab`.
- evaluation: `go test ./internal/evaluation`.
- runtime smoke: `go run ./cmd/sema-lab`의 list, text와 JSON 경로.
- full repository gate: `scripts/check.sh`.
