#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${VERSION:?VERSION is required}
dist_dir=${DIST_DIR:-dist}
targets=${TARGETS:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64"}

printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
  printf 'release build failed: VERSION must be a semantic version tag\n' >&2
  exit 1
}

case "$dist_dir" in
  /*) ;;
  *) dist_dir="$repo_root/$dist_dir" ;;
esac
case "$dist_dir" in
  /|"$repo_root")
    printf 'release build failed: unsafe DIST_DIR\n' >&2
    exit 1
    ;;
esac

mkdir -p "$dist_dir"
find "$dist_dir" -maxdepth 1 -type f -name 'sema-lab_*' -exec rm -f {} +
rm -f "$dist_dir/SHA256SUMS"

for target in $targets; do
  goos=${target%/*}
  goarch=${target#*/}
  if [ "$goos" = "$target" ] || [ -z "$goarch" ]; then
    printf 'release build failed: invalid target %s\n' "$target" >&2
    exit 1
  fi
  extension=
  if [ "$goos" = windows ]; then
    extension=.exe
  fi
  output="$dist_dir/sema-lab_${version}_${goos}_${goarch}${extension}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$version" \
    -o "$output" \
    ./cmd/sema-lab
done

(
  cd "$dist_dir"
  shasum -a 256 sema-lab_* >SHA256SUMS
)
