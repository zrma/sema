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
  for declaration in \
    '  stable_surface: service_wire_v1' \
    '  support_owner: repository_maintainers' \
    '  supported_previous_minor_releases: 1' \
    '  minimum_support_days: 180' \
    '  minimum_deprecation_minor_releases: 2' \
    '  compatibility_alias: v0alpha2' \
    '  compatibility_alias_status: supported' \
    '  compatibility_alias_end_of_support: not_scheduled'; do
    grep -Fxq "$declaration" docs/REPO_MANIFEST.yaml || {
      printf 'release admission failed: stable contract declaration is missing: %s\n' "$declaration" >&2
      exit 1
    }
  done
fi

scripts/check-release-notes.sh "$version"
if [ -n "${POSTGRES_REPORT_DIR:-}" ]; then
  scripts/check-postgres.sh "$POSTGRES_REPORT_DIR"
else
  scripts/check-postgres.sh
fi
if [ -n "${PERFORMANCE_REPORT_DIR:-}" ]; then
  scripts/check-performance.sh "$PERFORMANCE_REPORT_DIR"
else
  scripts/check-performance.sh
fi
scripts/check.sh
scripts/check-container.sh
scripts/check-release-build.sh
scripts/check-publication-boundary.py

printf 'sema release admission passed for %s\n' "$version"
