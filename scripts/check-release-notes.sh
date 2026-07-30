#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

version=${1:-}
printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
  printf 'release notes check failed: version must be an exact semantic version tag\n' >&2
  exit 2
}

read_manifest_value() {
  key=$1
  matches=$(sed -n "s/^  $key: //p" docs/REPO_MANIFEST.yaml)
  [ "$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')" -eq 1 ] || {
    printf 'release notes check failed: manifest must declare exactly one %s\n' "$key" >&2
    exit 1
  }
  printf '%s\n' "$matches"
}

current_version=$(read_manifest_value wire_compatibility_current)
[ "$version" = "$current_version" ] || {
  printf 'release notes check failed: version %s is not manifest current %s\n' "$version" "$current_version" >&2
  exit 1
}

notes=docs/release-notes.md
[ -s "$notes" ] &&
  [ "$(grep -Fxc "# Sema $version" "$notes")" -eq 1 ] &&
  grep -Fxq '## Stable Scope' "$notes" &&
  grep -Fxq '## Compatibility And Support' "$notes" &&
  grep -Fxq '## Migration And Rollback' "$notes" &&
  grep -Fxq '## Verification' "$notes" &&
  grep -Fxq '## Known Limits' "$notes" &&
  grep -Fq 'HTTP `/v1` service wire' "$notes" &&
  grep -Fq 'Go `github.com/zrma/sema/alpha` package' "$notes" &&
  grep -Fq 'at least 180 days' "$notes" &&
  grep -Fq 'at least two subsequent minor releases' "$notes" &&
  grep -Fq 'end of support is not scheduled' "$notes" &&
  grep -Fq 'docs/migrations/v0alpha2-to-v1.md' "$notes" &&
  grep -Fq 'does not integrate a real game service' "$notes" || {
  printf 'release notes check failed: stable scope, support, migration or known limits are incomplete\n' >&2
  exit 1
}

printf 'sema release notes check passed for %s\n' "$version"
