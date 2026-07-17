# Release Workflow

## Distribution Surfaces

- Go module source tag: `github.com/zrma/sema`와 public `alpha` package.
- `sema-lab` binaries: darwin amd64/arm64, linux amd64/arm64, windows amd64.
- `SHA256SUMS`: release artifact checksum list.

`scripts/build-release.sh`는 explicit `VERSION`을 binary에 주입하고 deterministic target names로 cross-build한다. `scripts/check-release-build.sh`는 host binary version과 checksum을 검증한다.

## Pre-Tag Gate

tag/release는 외부 write이며 사용자 승인 없이 실행하지 않는다. 승인 후에도 다음 순서를 지킨다.

1. `jj status`, intended bookmark와 remote commit을 확인한다.
2. `scripts/check.sh`와 `scripts/check-release-build.sh`를 통과한다.
3. Docker daemon 환경에서 `scripts/check-container.sh`를 통과한다.
4. live remote visibility와 release repository identity를 확인한다.
5. repository publication boundary gate와 권한 있는 machine-local private-inventory gate를 모두 통과한다.
6. release 대상 change의 attribution trailer, API marker, compatibility/migration document와 release note를 검토한다.
7. branch/bookmark가 remote에 반영된 뒤 같은 commit에 annotated `<version>` tag를 만든다.
8. tag push 뒤 GitHub workflow의 release와 artifact/checksum을 재조회한다.

CI는 tag가 공개된 뒤의 backstop이므로 machine-local inventory gate를 대체하지 않는다.

## Automation

`v*` tag push는 `.github/workflows/release.yml`을 실행한다.

1. 전체 repository와 container check.
2. five-target `sema-lab` cross-build와 checksum 생성.
3. `gh release create --verify-tag`로 GitHub Release와 artifact 게시.

build script는 semantic-version-shaped `VERSION`만 허용한다. release가 이미 존재하거나 tag 검증이 실패하면 workflow는 덮어쓰지 않고 실패한다.

## Post-Release Verification

- remote tag와 release가 같은 commit인지 확인한다.
- 모든 target artifact와 `SHA256SUMS`가 존재하는지 확인한다.
- host artifact checksum과 `sema-lab -version`을 검증한다.
- Go consumer가 tagged module에서 `alpha.Compose` example을 build/test할 수 있는지 확인한다.
- release note가 alpha compatibility와 known limitations를 정확히 설명하는지 확인한다.

현재 workflow만 준비되어 있으며 이 문서 작성 시점에 release/tag publish를 수행했다는 뜻은 아니다.
