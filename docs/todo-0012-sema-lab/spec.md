# P5 Sema Lab Spec

- Status: Complete

## Objective

안정적인 public SDK나 production transport를 열기 전에 reference workload를 직접 실행하고 proposal placement, coverage와 search evidence를 관찰할 수 있는 deterministic evaluation entrypoint를 제공한다.

## Deliverables

- `cmd/sema-lab` executable과 testable command boundary.
- team, battle royale, backfill, no-match, latency와 objective relaxation built-in corpus.
- ticket/player coverage, unmatched reason와 search/quality evidence report.
- canonical text output, optional placement detail과 experimental `v0alpha1` JSON.
- runtime smoke가 포함된 repository gate와 사용 문서.

## Acceptance

- workload 인자 생략은 전체 corpus, 명시는 선택 corpus만 실행한다.
- workload 순서와 중복은 ordered report에 영향을 주지 않는다.
- 같은 fixed workload를 반복하면 proposal/team placement와 report가 같다.
- 2:2부터 50:50 team matrix, 100-player duo/squad, backfill과 no-match 결과가 player coverage까지 검증된다.
- role-quality와 wait relaxation evidence, latency hard rejection과 stable unmatched reason이 report에 나타난다.
- text summary, detail과 JSON 경로가 실제 command smoke를 통과한다.
- focused tests, race detector와 `scripts/check.sh`가 통과한다.

## Out Of Scope

- 외부 workload file/schema와 production traffic ingestion.
- stable JSON/CLI compatibility guarantee와 public Go SDK.
- machine-independent latency/allocation threshold.
- policy winner activation, coordinator mutation, network와 storage.

## Completion Evidence

- `internal/lab`이 built-in workload를 defensive copy로 제공하고 `internal/simulation`을 통해 side effect 없이 실행한다.
- report가 policy fingerprint, proposal/team placement, ticket/player coverage, unmatched와 search evidence를 함께 보존한다.
- command test가 list, deterministic text/detail, JSON과 invalid input exit behavior를 검증한다.
- full corpus smoke와 repository gate가 통과한다.

사용법과 report 경계는 `docs/sema-lab.md`, 장기 순서는 ADR 0006이 소유한다.
