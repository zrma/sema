#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

version=${1:-v0.0.0-test}
printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
  printf 'release admission failed: version must be a semantic version tag\n' >&2
  exit 2
}

major=${version#v}
major=${major%%.*}
if [ "$major" -eq 0 ]; then
  grep -Fxq '  alpha_admitted: true' docs/REPO_MANIFEST.yaml || {
    printf 'release admission failed: alpha release is blocked; see docs/release-admission.md\n' >&2
    exit 1
  }
else
  grep -Fxq '  stable_admitted: true' docs/REPO_MANIFEST.yaml || {
    printf 'release admission failed: stable release is blocked; see docs/release-admission.md\n' >&2
    exit 1
  }
fi

scripts/check.sh
scripts/check-container.sh
if [ -n "${PERFORMANCE_REPORT_DIR:-}" ]; then
  scripts/check-performance.sh "$PERFORMANCE_REPORT_DIR"
else
  scripts/check-performance.sh
fi
scripts/check-release-build.sh
scripts/check-publication-boundary.py

printf 'sema release admission passed for %s\n' "$version"
