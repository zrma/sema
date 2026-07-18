#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

image="postgres:17.10-alpine3.24@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"
container="sema-postgres-check-$$"
password="sema-integration-only"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

command -v docker >/dev/null 2>&1 || {
  printf 'postgres check failed: docker is required\n' >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  printf 'postgres check failed: docker daemon is unavailable\n' >&2
  exit 1
}

docker run -d \
  --name "$container" \
  -e POSTGRES_DB=sema_test \
  -e POSTGRES_PASSWORD="$password" \
  -p 127.0.0.1::5432 \
  "$image" >/dev/null

attempt=0
until docker exec "$container" pg_isready -U postgres -d sema_test >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    printf 'postgres check failed: database did not become ready\n' >&2
    exit 1
  fi
  sleep 1
done

address=$(docker port "$container" 5432/tcp | sed -n '1p')
[ -n "$address" ] || {
  printf 'postgres check failed: published address is missing\n' >&2
  exit 1
}

SEMA_POSTGRES_TEST_DSN="postgres://postgres:${password}@${address}/sema_test?sslmode=disable" \
  go test -race ./internal/repository/postgres ./internal/service ./internal/targetapi

printf 'sema postgres repository check passed\n'
