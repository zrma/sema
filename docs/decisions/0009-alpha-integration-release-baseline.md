# ADR 0009: Alpha Integration And Release Baseline

- Status: Accepted

## Context

P5-P7은 executable evaluation과 core evidence를 제공하지만 모든 Go package가 `internal/`이라 외부 repository가 composition engine을 import할 수 없다. 실제 production consumer는 아직 없으므로 전체 lifecycle이나 service protocol을 안정화할 증거도 없다.

## Decision

- 첫 public import path는 `github.com/zrma/sema/alpha`다.
- public surface는 immutable snapshot을 side-effect-free proposal batch로 변환하는 `Compose`만 제공한다.
- public type은 internal type alias가 아니라 explicit copy/conversion boundary다.
- repository-owned `examples/compose`를 첫 reference consumer로 사용한다.
- coordinator/reservation/assignment와 transport/storage는 public surface에 포함하지 않는다.
- `sema-lab`은 versioned binaries/checksum으로 배포할 수 있게 준비하되 실제 release는 별도 승인 경계다.

## Consequences

- 외부 consumer는 core composition을 import할 수 있지만 alpha source compatibility는 보장되지 않는다.
- stable API는 실제 consumer evidence가 생길 때 alpha usage에서 추출한다.
- release workflow가 존재해도 tag push 전 local publication/private-inventory gate 책임은 남는다.
- JSON representation은 diagnostic이며 wire protocol이 아니다.

## Revisit Triggers

- 실제 consumer가 reservation/assignment lifecycle을 같은 process에서 요구한다.
- separate-process consumer가 versioned wire protocol을 요구한다.
- alpha API가 두 release cycle 이상 안정되고 v1 support commitment가 생긴다.
- binary distribution에 signing, provenance 또는 package registry가 필요하다.
