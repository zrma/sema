#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
temporary_root=$(mktemp -d)
fixture_pid=
temporary_report=false

cleanup() {
  if [ -n "$fixture_pid" ]; then
    kill -TERM "$fixture_pid" >/dev/null 2>&1 || true
    wait "$fixture_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$temporary_root"
  if [ "$temporary_report" = true ]; then
    rm -rf "$report_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

usage() {
  printf 'usage: scripts/check-wire-compatibility-matrix.sh <previous-tag> <current-tag> [report-directory]\n' >&2
  printf '       scripts/check-wire-compatibility-matrix.sh --self-test [report-directory]\n' >&2
}

semantic_tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
self_test=false
case "${1:-}" in
  --self-test)
    [ "$#" -le 2 ] || {
      usage
      exit 2
    }
    self_test=true
    previous_version=self-test-previous
    current_version=self-test-current
    report_argument=${2:-}
    ;;
  "")
    usage
    exit 2
    ;;
  *)
    [ "$#" -ge 2 ] && [ "$#" -le 3 ] || {
      usage
      exit 2
    }
    previous_version=$1
    current_version=$2
    report_argument=${3:-}
    printf '%s\n' "$previous_version" | grep -Eq "$semantic_tag_pattern" || {
      printf 'wire compatibility matrix failed: previous tag must be a semantic version\n' >&2
      exit 2
    }
    printf '%s\n' "$current_version" | grep -Eq "$semantic_tag_pattern" || {
      printf 'wire compatibility matrix failed: current tag must be a semantic version\n' >&2
      exit 2
    }
    [ "$previous_version" != "$current_version" ] || {
      printf 'wire compatibility matrix failed: previous and current tags must be distinct\n' >&2
      exit 2
    }
    ;;
esac

if [ -n "$report_argument" ]; then
  report_dir=$report_argument
  mkdir -p "$report_dir"
else
  report_dir=$(mktemp -d)
  temporary_report=true
fi
report_dir=$(CDPATH= cd -- "$report_dir" && pwd)
case "$report_dir" in
  /|"$repo_root")
    printf 'wire compatibility matrix failed: unsafe report directory\n' >&2
    exit 2
    ;;
esac
report="$report_dir/wire-compatibility-matrix.json"
[ ! -e "$report" ] || {
  printf 'wire compatibility matrix failed: report already exists\n' >&2
  exit 2
}

previous_source="$temporary_root/previous-source"
current_source="$temporary_root/current-source"
mkdir -p "$previous_source" "$current_source" "$temporary_root/bin" "$temporary_root/run"

if [ "$self_test" = true ]; then
  previous_commit=self-test
  current_commit=self-test
  previous_source=$repo_root
  current_source=$repo_root
  report_schema=sema.wire-compatibility-matrix-self-test.v1
else
  command -v git >/dev/null 2>&1 || {
    printf 'wire compatibility matrix failed: git is required to resolve tags\n' >&2
    exit 1
  }
  command -v tar >/dev/null 2>&1 || {
    printf 'wire compatibility matrix failed: tar is required to extract tags\n' >&2
    exit 1
  }
  for tag in "$previous_version" "$current_version"; do
    git show-ref --verify --quiet "refs/tags/$tag" || {
      printf 'wire compatibility matrix failed: required local tag is missing: %s\n' "$tag" >&2
      exit 1
    }
    [ "$(git cat-file -t "refs/tags/$tag")" = tag ] || {
      printf 'wire compatibility matrix failed: tag must be annotated: %s\n' "$tag" >&2
      exit 1
    }
  done
  previous_commit=$(git rev-parse "refs/tags/$previous_version^{commit}")
  current_commit=$(git rev-parse "refs/tags/$current_version^{commit}")
  [ "$previous_commit" != "$current_commit" ] || {
    printf 'wire compatibility matrix failed: tags must resolve to distinct commits\n' >&2
    exit 1
  }
  git archive --format=tar "$previous_version" | tar -xf - -C "$previous_source"
  git archive --format=tar "$current_version" | tar -xf - -C "$current_source"
  report_schema=sema.wire-compatibility-matrix.v1
fi

build_pair() {
  source_dir=$1
  version=$2
  prefix=$3
  (
    cd "$source_dir"
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$temporary_root/bin/${prefix}-fixture" \
      ./cmd/sema-wire-fixture
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$temporary_root/bin/${prefix}-client" \
      ./cmd/sema-conformance
  )
}

build_pair "$previous_source" "$previous_version" previous
build_pair "$current_source" "$current_version" current

write_token=wire-matrix-write-token
read_token=wire-matrix-read-token
other_token=wire-matrix-other-token

stop_fixture() {
  [ -n "$fixture_pid" ] || return 0
  kill -TERM "$fixture_pid"
  wait "$fixture_pid"
  fixture_pid=
}

run_direction() {
  client_prefix=$1
  service_prefix=$2
  direction=$3
  ready_log="$temporary_root/run/${direction}-ready.log"
  error_log="$temporary_root/run/${direction}-fixture-error.log"
  client_report="$temporary_root/run/${direction}-client.json"
  client_error="$temporary_root/run/${direction}-client-error.log"

  SEMA_TARGET_WRITE_TOKEN="$write_token" \
    SEMA_TARGET_READ_TOKEN="$read_token" \
    SEMA_TARGET_OTHER_TENANT_TOKEN="$other_token" \
    "$temporary_root/bin/${service_prefix}-fixture" \
    -listen 127.0.0.1:0 >"$ready_log" 2>"$error_log" &
  fixture_pid=$!

  attempt=0
  address=
  while [ "$attempt" -lt 10 ]; do
    address=$(sed -n 's/^sema-wire-fixture listening on //p' "$ready_log" | sed -n '1p')
    [ -z "$address" ] || break
    kill -0 "$fixture_pid" >/dev/null 2>&1 || {
      printf 'wire compatibility matrix failed: %s fixture exited before readiness\n' "$direction" >&2
      exit 1
    }
    attempt=$((attempt + 1))
    sleep 1
  done
  case "$address" in
    127.0.0.1:*) ;;
    *)
      printf 'wire compatibility matrix failed: %s fixture did not publish a loopback address\n' "$direction" >&2
      exit 1
      ;;
  esac

  SEMA_TARGET_WRITE_TOKEN="$write_token" \
    SEMA_TARGET_READ_TOKEN="$read_token" \
    SEMA_TARGET_OTHER_TENANT_TOKEN="$other_token" \
    "$temporary_root/bin/${client_prefix}-client" \
    -base-url "http://$address" -allow-http -timeout 15s \
    >"$client_report" 2>"$client_error"
  grep -Fq '"schema":"sema.wire-conformance.v1"' "$client_report" &&
    grep -Fq '"lifecycle_complete":true' "$client_report" || {
      printf 'wire compatibility matrix failed: %s conformance report is incomplete\n' "$direction" >&2
      exit 1
    }
  if grep -Fq "$write_token" "$ready_log" "$error_log" "$client_report" "$client_error" ||
    grep -Fq "$read_token" "$ready_log" "$error_log" "$client_report" "$client_error" ||
    grep -Fq "$other_token" "$ready_log" "$error_log" "$client_report" "$client_error"; then
    printf 'wire compatibility matrix failed: %s output exposed a fixture token\n' "$direction" >&2
    exit 1
  fi
  stop_fixture
}

run_direction previous current previous-client-current-service
run_direction current previous current-client-previous-service

printf '%s\n' \
  '{' \
  "  \"schema\": \"$report_schema\"," \
  '  "previous": {' \
  "    \"version\": \"$previous_version\"," \
  "    \"commit\": \"$previous_commit\"" \
  '  },' \
  '  "current": {' \
  "    \"version\": \"$current_version\"," \
  "    \"commit\": \"$current_commit\"" \
  '  },' \
  '  "directions": [' \
  '    {' \
  '      "client": "previous",' \
  '      "service": "current",' \
  '      "passed": true' \
  '    },' \
  '    {' \
  '      "client": "current",' \
  '      "service": "previous",' \
  '      "passed": true' \
  '    }' \
  '  ]' \
  '}' >"$report"

printf 'sema wire compatibility matrix passed: %s -> %s\n' "$previous_version" "$current_version"
