#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
temporary_report=false

cleanup() {
  if [ "$temporary_report" = true ]; then
    rm -rf "$report_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

usage() {
  printf 'usage: scripts/check-wire-compatibility-release.sh <current-tag> <report-directory>\n' >&2
  printf '       scripts/check-wire-compatibility-release.sh --self-test [report-directory]\n' >&2
}

read_manifest_value() {
  key=$1
  matches=$(sed -n "s/^  $key: //p" docs/REPO_MANIFEST.yaml)
  [ "$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')" -eq 1 ] || {
    printf 'wire compatibility release failed: manifest must declare exactly one %s\n' "$key" >&2
    exit 1
  }
  printf '%s\n' "$matches"
}

previous_version=$(read_manifest_value wire_compatibility_previous)
current_version=$(read_manifest_value wire_compatibility_current)
semantic_tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
printf '%s\n' "$previous_version" | grep -Eq "$semantic_tag_pattern" &&
  printf '%s\n' "$current_version" | grep -Eq "$semantic_tag_pattern" &&
  [ "$previous_version" != "$current_version" ] || {
  printf 'wire compatibility release failed: manifest pair must contain distinct semantic tags\n' >&2
  exit 1
}

self_test=false
case "${1:-}" in
  --self-test)
    [ "$#" -le 2 ] || {
      usage
      exit 2
    }
    self_test=true
    report_argument=${2:-}
    ;;
  "")
    usage
    exit 2
    ;;
  *)
    [ "$#" -eq 2 ] || {
      usage
      exit 2
    }
    [ "$1" = "$current_version" ] || {
      printf 'wire compatibility release failed: tag %s is not manifest current %s\n' "$1" "$current_version" >&2
      exit 1
    }
    report_argument=$2
    ;;
esac

if [ -n "$report_argument" ]; then
  report_dir=$report_argument
else
  report_dir=$(mktemp -d)
  temporary_report=true
fi

if [ "$self_test" = true ]; then
  scripts/check-wire-compatibility-matrix.sh --self-test "$report_dir"
  expected_schema=sema.wire-compatibility-matrix-self-test.v1
else
  scripts/check-wire-compatibility-matrix.sh "$previous_version" "$current_version" "$report_dir"
  expected_schema=sema.wire-compatibility-matrix.v1
fi

report="$report_dir/wire-compatibility-matrix.json"
[ -f "$report" ] &&
  grep -Fxq "  \"schema\": \"$expected_schema\"," "$report" &&
  [ "$(grep -Fc '      "passed": true' "$report")" -eq 2 ] || {
  printf 'wire compatibility release failed: matrix report is missing or incomplete\n' >&2
  exit 1
}

if [ "$self_test" = false ]; then
  grep -Fxq "    \"version\": \"$previous_version\"," "$report" &&
    grep -Fxq "    \"version\": \"$current_version\"," "$report" || {
    printf 'wire compatibility release failed: matrix report does not match manifest pair\n' >&2
    exit 1
  }
fi

printf 'sema wire compatibility release gate passed: %s -> %s\n' "$previous_version" "$current_version"
