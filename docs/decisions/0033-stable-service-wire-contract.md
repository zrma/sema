# ADR 0033: Stable Service Wire Contract

## Status

Accepted

## Context

P31은 PostgreSQL authority, stateless replica, provider-neutral OIDC, workload, failure와 recovery evidence를 완료했다. P32는 실제 게임 연동이나 기존 매치메이커 교체가 아니라 이 공통 프레임워크가 장기간 유지할 compatibility 책임을 확정해야 한다.

`v0.3.0`과 `v0.4.0`은 authenticated service wire를 포함하며, repository-owned matrix가 previous client와 current service를 양방향으로 실행한다. public Go `alpha` package는 side-effect-free composition을 제공하지만 아직 별도의 stable consumer history와 compatibility evidence가 없다.

## Decision

- stable public surface는 HTTP service wire의 `/v1` lifecycle로 한정한다.
- public Go `alpha` package, `internal/` package, diagnostic report, metrics와 deployment-specific configuration은 stable service wire 약속에 포함하지 않는다.
- `/v1`은 `/v0alpha2`와 같은 PostgreSQL authority, authentication, authorization, tenant isolation과 resource lifecycle을 사용한다. response envelope marker는 요청한 prefix에 맞춰 각각 `v1`과 `v0alpha2`를 반환한다.
- 같은 stable major의 변경은 기존 request/response decoder와 documented behavior를 보존하는 additive, backward-compatible 변경만 허용한다.
- stable successor minor가 출고되면 최신 minor와 바로 이전 stable minor를 successor release 후 최소 180일 동안 지원한다.
- deprecated route, field 또는 command alias는 deprecation을 공지한 뒤 최소 두 번의 후속 minor release와 180일 중 더 긴 기간 동안 유지한다.
- critical authentication, tenant isolation 또는 data-integrity security defect만 위 기간보다 이른 호환성 변경의 예외가 될 수 있다. 예외 release는 advisory, migration, rollback limitation, regression evidence와 end-of-support 신호를 함께 제공해야 한다.
- support owner는 repository maintainers다.
- `/v0alpha2` compatibility route는 `v1.0.0` 이후 최소 180일과 두 번의 후속 minor release 중 더 긴 기간 동안 유지한다. 제거 release 전에 명시적인 end-of-support 신호와 migration evidence를 제공한다.
- 실제 consumer, production traffic, predecessor deployment 또는 cutover rehearsal은 stable admission의 필수 조건이 아니다.

## Consequences

`sema-conformance`는 기본적으로 `/v1`을 검증한다. legacy `sema-target-smoke`와 tagged predecessor matrix는 `/v0alpha2`를 명시해 호환 경계를 검증한다. 두 prefix가 같은 state를 관찰하므로 compatibility route가 별도 authority나 divergent lifecycle을 만들지 않는다.

Go module의 `v1.0.0` source tag는 service wire release vehicle이지만 `github.com/zrma/sema/alpha`를 stable Go API로 승격하지 않는다. stable Go package가 필요하면 별도 milestone, public package와 consumer compatibility evidence가 필요하다.

## Rejected Alternatives

- Go `alpha`까지 stable scope에 포함: tagged consumer evidence와 확정된 source contract가 부족하다.
- `/v0alpha2`를 marker만 바꿔 즉시 제거: supported predecessor client와 rollback path를 없앤다.
- 실제 게임 integration을 admission 조건으로 추가: standalone framework의 repository-owned readiness를 특정 consumer 일정에 종속시킨다.

## Verification

- current client는 `/v1` full lifecycle을 실행한다.
- previous client는 current service의 `/v0alpha2` alias를 실행한다.
- current client는 supported previous service에 `/v0alpha2`를 명시해 실행한다.
- 같은 resource가 `/v1`과 `/v0alpha2`에서 공유되고 각 response marker가 요청 prefix와 일치한다.
- release admission은 scope, support owner, numeric window, migration document와 exact tagged matrix pair를 기계적으로 확인한다.
