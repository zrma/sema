# P8 Public Integration And Distribution Spec

- Status: Complete

## Objective

internal implementation을 직접 노출하지 않고 실제 import/build 가능한 최소 alpha composition API, repository-owned consumer와 검증 가능한 distribution workflow를 제공한다.

## Deliverables

- public `github.com/zrma/sema/alpha` types, `Compose`와 typed error boundary.
- internal alias가 아닌 explicit input/output conversion.
- internal import 없이 alpha package만 사용하는 `examples/compose` consumer.
- alpha compatibility/migration policy와 stable API gate.
- versioned `sema-lab` cross-build/checksum scripts와 tag-triggered release workflow.

## Acceptance

- 외부 package test가 deterministic multi-match, typed invalid input과 candidate-window evidence를 검증한다.
- public input mutation이 internal planner로 새지 않는다.
- example consumer가 build/run되고 proposal batch JSON을 출력한다.
- `sema-lab -version`은 build-time version injection을 반영한다.
- host release artifact가 실행되고 checksum verification을 통과한다.
- release workflow는 full gate 뒤 verified tag에만 GitHub Release를 만든다.
- compatibility/migration과 pre/post release gate가 문서화된다.
- full test/race/repository/publication gate가 통과한다.

## Out Of Scope

- stable/v1 API guarantee와 production wire schema.
- public coordinator/reservation/assignment lifecycle.
- 이번 change에서 실제 tag, GitHub Release 또는 remote push 생성.
- package registry, container image와 signed provenance.

## Completion Evidence

- `go test ./alpha`와 `go run ./examples/compose`가 external surface를 통과한다.
- repository source scan이 example의 direct `internal/` import를 거부한다.
- `scripts/check-release-build.sh`가 embedded version과 checksum을 검증한다.
- `scripts/check.sh`가 public consumer/release smoke와 기존 전체 gate를 함께 실행한다.

public API는 `docs/public-api.md`, compatibility는 `docs/api-compatibility.md`, release는 `docs/releasing.md`, architecture decision은 ADR 0009가 소유한다.
