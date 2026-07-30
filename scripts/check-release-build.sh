#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dist_dir=$(mktemp -d)
trap 'rm -rf "$dist_dir"' EXIT HUP INT TERM

goos=$(go env GOOS)
goarch=$(go env GOARCH)
version=v0.0.0-test
VERSION="$version" TARGETS="$goos/$goarch" DIST_DIR="$dist_dir" "$repo_root/scripts/build-release.sh"
if VERSION=v1.2.3oops TARGETS="$goos/$goarch" DIST_DIR="$dist_dir" "$repo_root/scripts/build-release.sh" >/dev/null 2>&1; then
  printf 'release build check failed: malformed version was accepted\n' >&2
  exit 1
fi

for command in sema-lab sema-service sema-conformance sema-postgres-migrate; do
  binary="$dist_dir/${command}_${version}_${goos}_${goarch}"
  if [ "$goos" = windows ]; then
    binary="$binary.exe"
  fi
  [ -f "$binary" ] || {
    printf 'release build check failed: host artifact is missing: %s\n' "$command" >&2
    exit 1
  }
  if [ "$goos" != windows ]; then
    "$binary" -version | grep -Fxq "$command $version" || {
      printf 'release build check failed: embedded version is incorrect: %s\n' "$command" >&2
      exit 1
    }
  fi
done
(
  cd "$dist_dir"
  shasum -a 256 -c SHA256SUMS >/dev/null
)

printf 'sema release build check passed\n'
