# Release Workflow

## Distribution Surfaces

- Go module source tag: `github.com/zrma/sema`와 public `alpha` package.
- `sema-lab`, `sema-service`, `sema-conformance`와 `sema-postgres-migrate` binaries: darwin amd64/arm64, linux amd64/arm64, windows amd64.
- `SHA256SUMS`: release artifact checksum list.
- `Dockerfile`: local/reproducible deployment artifact이며 현재 public registry publication surface는 아니다.

`scripts/build-release.sh`는 explicit `VERSION`을 binary에 주입하고 deterministic target names로 cross-build한다. `scripts/check-release-build.sh`는 네 public command의 host binary version과 checksum을 검증한다. `sema-wire-fixture`는 tagged source에서 cross-version gate가 빌드하는 loopback-only test command이며 release artifact가 아니다.

## Pre-Tag Gate

tag/release는 외부 write이며 사용자 승인 없이 실행하지 않는다. 승인 후에도 다음 순서를 지킨다.

1. `jj status`, intended bookmark와 remote commit을 확인한다.
2. `scripts/check-release-admission.sh <version>`을 통과한다.
3. live remote visibility와 release repository identity를 확인한다.
4. 권한 있는 machine-local private-inventory gate를 통과한다.
5. release 대상 change의 attribution trailer, API marker, compatibility/migration document와 release note를 검토한다.
6. branch/bookmark가 remote에 반영된 뒤 같은 commit에 annotated `<version>` tag를 만든다.
7. tag push 뒤 GitHub workflow의 release와 artifact/checksum을 재조회한다.

cross-version evidence를 만드는 alpha release는 signed annotated tag를 사용한다. 첫 pair는 `v0.3.0`에서 service distribution과 tagged-source fixture를 고정하고 `v0.4.0`에서 양방향 matrix를 실행하는 순서다. 두 tag는 각각 대상 commit의 별도 publication 승인 아래 출고한다. release workflow checkout은 이전 tag source를 함께 가져오며 `scripts/check-wire-compatibility-release.sh`가 manifest의 exact previous/current pair를 실행한다. manifest의 current와 다른 tag는 release workflow에서 실패하므로 새 release마다 pair를 명시적으로 갱신해야 한다. `--self-test` 결과를 tagged evidence로 승격하지 않는다.

CI는 tag가 공개된 뒤의 backstop이므로 machine-local inventory gate를 대체하지 않는다.

## Automation

`v*` tag push는 `.github/workflows/release.yml`을 실행한다.

1. version-aware release admission이 repository, container, repeated performance/recovery와 publication gate를 실행한다.
2. manifest의 previous/current tag source로 양방향 wire matrix를 실행하고 bounded JSON evidence를 만든다.
3. five-target public command cross-build와 matrix evidence를 포함한 checksum을 생성한다.
4. `gh release create --verify-tag`로 GitHub Release, matrix evidence와 artifact를 게시한다.

build script는 semantic-version-shaped `VERSION`만 허용한다. release가 이미 존재하거나 tag 검증이 실패하면 workflow는 덮어쓰지 않고 실패한다.

현재 manifest는 v0 alpha만 admit한다. major version 1 이상 tag는 `stable_admitted: true` 전에는 workflow에서 실패한다. ADR 0033은 stable 범위를 `/v1` service wire로 한정했으며 남은 migration/release gate와 변경 절차는 `docs/release-admission.md`가 소유한다.

GitHub release에서 직접 받은 Unix binary는 archive가 아니므로 executable mode가 보존되지 않을 수 있다. checksum 검증 뒤 실행할 artifact에 `chmod +x <binary>`를 적용한다. 이 packaging 특성은 checksum이나 binary identity 실패가 아니다.

## Post-Release Verification

- remote tag와 release가 같은 commit인지 확인한다.
- 네 command의 모든 target artifact와 `SHA256SUMS`가 존재하는지 확인한다.
- `wire-compatibility-matrix.json`의 previous/current tag와 commit이 remote tag에 일치하고 두 방향이 모두 통과했는지 확인한다.
- host artifact checksum과 네 public command의 `-version`을 검증한다.
- Go consumer가 tagged module에서 `alpha.Compose` example을 build/test할 수 있는지 확인한다.
- release note가 alpha compatibility와 known limitations를 정확히 설명하는지 확인한다.

현재 workflow만 준비되어 있으며 이 문서 작성 시점에 release/tag publish를 수행했다는 뜻은 아니다.
