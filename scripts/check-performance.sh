#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

if [ "$#" -gt 1 ]; then
  printf 'usage: scripts/check-performance.sh [report-directory]\n' >&2
  exit 2
fi

temporary_reports=false
if [ "$#" -eq 1 ]; then
  report_dir=$1
  mkdir -p "$report_dir"
else
  report_dir=$(mktemp -d)
  temporary_reports=true
fi
report_dir=$(CDPATH= cd -- "$report_dir" && pwd)
case "$report_dir" in
  /|"$repo_root")
    printf 'performance check failed: unsafe report directory\n' >&2
    exit 2
    ;;
esac

for report in profile.json go-benchmarks.json service-run-01.json service-run-02.json service-run-03.json; do
  [ ! -e "$report_dir/$report" ] || {
    printf 'performance check failed: report already exists: %s\n' "$report" >&2
    exit 2
  }
done

raw_benchmarks=$(mktemp)
benchmark_image="sema-benchmark-check-$$:local"
runtime_image="sema-performance-check-$$:local"
container=
volume=

cleanup() {
  if [ -n "$container" ]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  if [ -n "$volume" ]; then
    docker volume rm -f "$volume" >/dev/null 2>&1 || true
  fi
  docker image rm -f "$benchmark_image" "$runtime_image" >/dev/null 2>&1 || true
  rm -f "$raw_benchmarks"
  if [ "$temporary_reports" = true ]; then
    rm -rf "$report_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

command -v docker >/dev/null 2>&1 || {
  printf 'performance check failed: docker is required\n' >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  printf 'performance check failed: docker daemon is unavailable\n' >&2
  exit 1
}

docker build --pull=false --target build --build-arg VERSION=v0.0.0-test -t "$benchmark_image" . >/dev/null
: >"$raw_benchmarks"

docker run --rm --cpus 2 --memory 2g --pids-limit 512 --network none "$benchmark_image" \
  go test ./internal/planner -run '^$' -bench '^BenchmarkPlan' -benchmem -benchtime=3x -count=5 \
  >>"$raw_benchmarks"
docker run --rm --cpus 2 --memory 2g --pids-limit 512 --network none "$benchmark_image" \
  go test ./internal/engine -run '^$' -bench '^BenchmarkEngine' -benchmem -benchtime=3x -count=5 \
  >>"$raw_benchmarks"
docker run --rm --cpus 2 --memory 2g --pids-limit 512 --network none "$benchmark_image" \
  go test ./internal/durable -run '^$' -bench '^BenchmarkOpenReplay$' -benchmem -benchtime=3x -count=3 \
  >>"$raw_benchmarks"

go run ./cmd/sema-benchmark-gate -minimum-samples 3 <"$raw_benchmarks" >"$report_dir/go-benchmarks.json"

docker build --pull=false --build-arg VERSION=v0.0.0-test -t "$runtime_image" . >/dev/null
run=1
while [ "$run" -le 3 ]; do
  suffix=$(printf '%02d' "$run")
  volume="sema-performance-check-$$-$suffix"
  container="sema-performance-check-$$-$suffix"
  docker volume create "$volume" >/dev/null
  docker run --name "$container" --rm \
    --cpus 2 \
    --memory 2g \
    --pids-limit 256 \
    --network none \
    --read-only \
    --cap-drop ALL \
    --security-opt no-new-privileges:true \
    --env TMPDIR=/var/lib/sema \
    --mount "type=volume,source=$volume,target=/var/lib/sema" \
    --entrypoint /usr/local/bin/sema-ops-check \
    "$runtime_image" \
    -cycles 10 \
    -tickets-per-cycle 20 \
    -concurrency 8 \
    -timeout 30s \
    -max-p95 250ms \
    -max-request 1s \
    -max-duration 30s \
    -format jsonl \
    >"$report_dir/service-run-$suffix.json"
  container=
  docker volume rm -f "$volume" >/dev/null
  volume=
  run=$((run + 1))
done

printf '%s\n' \
  '{"schema_version":"sema-target-profile-v1","profile":"sema-reference-container-v1","platform":"linux/native","go_version":"1.26.0","cpu_limit":2,"memory_limit_mib":2048,"benchmark_minimum_samples":3,"operational_runs":3,"cycles_per_run":10,"tickets_per_cycle":20,"concurrency":8,"p95_budget_millis":250,"max_request_budget_millis":1000,"run_duration_budget_millis":30000}' \
  >"$report_dir/profile.json"

printf 'sema performance profile passed\n'
