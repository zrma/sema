#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${VERSION:?VERSION is required}
dist_dir=${DIST_DIR:-dist}
targets=${TARGETS:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64"}
commands="sema-lab sema-service sema-conformance sema-postgres-migrate"
wire_compatibility_report=${WIRE_COMPATIBILITY_REPORT:-}

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
for command in $commands; do
  find "$dist_dir" -maxdepth 1 -type f -name "${command}_*" -exec rm -f {} +
done
rm -f "$dist_dir/SHA256SUMS" "$dist_dir/wire-compatibility-matrix.json"

if [ -n "$wire_compatibility_report" ]; then
  [ -f "$wire_compatibility_report" ] || {
    printf 'release build failed: wire compatibility report is missing\n' >&2
    exit 1
  }
  grep -Fxq '  "schema": "sema.wire-compatibility-matrix.v1",' "$wire_compatibility_report" &&
    grep -Fxq "    \"version\": \"$version\"," "$wire_compatibility_report" &&
    [ "$(grep -Fc '      "passed": true' "$wire_compatibility_report")" -eq 2 ] || {
      printf 'release build failed: wire compatibility report is invalid for %s\n' "$version" >&2
      exit 1
    }
  cp "$wire_compatibility_report" "$dist_dir/wire-compatibility-matrix.json"
fi

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
  for command in $commands; do
    output="$dist_dir/${command}_${version}_${goos}_${goarch}${extension}"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$output" \
      "./cmd/$command"
  done
done

(
  cd "$dist_dir"
  if [ -f wire-compatibility-matrix.json ]; then
    shasum -a 256 \
      sema-lab_* \
      sema-service_* \
      sema-conformance_* \
      sema-postgres-migrate_* \
      wire-compatibility-matrix.json >SHA256SUMS
  else
    shasum -a 256 \
      sema-lab_* \
      sema-service_* \
      sema-conformance_* \
      sema-postgres-migrate_* >SHA256SUMS
  fi
)
