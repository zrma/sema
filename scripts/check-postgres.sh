#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

image="postgres:17.10-alpine3.24@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"
container="sema-postgres-check-$$"
password="sema-integration-only"
rehearsal_schema="sema_backup_rehearsal"
rehearsal_directory=""
container_dump="/tmp/sema-backup-rehearsal.dump"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  if [ -n "$rehearsal_directory" ]; then
    rm -f -- "$rehearsal_directory/v0.journal" "$rehearsal_directory/manifest.json"
    rmdir -- "$rehearsal_directory" >/dev/null 2>&1 || true
  fi
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

rehearsal_directory=$(mktemp -d "${TMPDIR:-/tmp}/sema-postgres-rehearsal.XXXXXX")
rehearsal_journal="$rehearsal_directory/v0.journal"
rehearsal_manifest="$rehearsal_directory/manifest.json"
rehearsal_dsn="postgres://postgres:${password}@${address}/sema_test?sslmode=disable"

docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d sema_test \
  -c "CREATE SCHEMA $rehearsal_schema" >/dev/null
SEMA_POSTGRES_TEST_DSN="$rehearsal_dsn" go run ./cmd/sema-postgres-rehearsal \
  -phase seed -schema "$rehearsal_schema" \
  -journal "$rehearsal_journal" -manifest "$rehearsal_manifest"

docker exec "$container" pg_dump -U postgres -d sema_test \
  --format=custom --no-owner --no-privileges \
  --schema="$rehearsal_schema" --file="$container_dump"
docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d sema_test \
  -c "SET client_min_messages TO warning; DROP SCHEMA $rehearsal_schema CASCADE" >/dev/null
docker exec "$container" pg_restore -U postgres -d sema_test \
  --exit-on-error --no-owner --no-privileges "$container_dump"

SEMA_POSTGRES_TEST_DSN="$rehearsal_dsn" go run ./cmd/sema-postgres-rehearsal \
  -phase verify -schema "$rehearsal_schema" \
  -journal "$rehearsal_journal" -manifest "$rehearsal_manifest"

docker exec "$container" psql -v ON_ERROR_STOP=1 -U postgres -d sema_test \
  -c "SET client_min_messages TO warning; DROP SCHEMA $rehearsal_schema CASCADE" >/dev/null
go run ./cmd/sema-postgres-rehearsal \
  -phase rollback -journal "$rehearsal_journal" -manifest "$rehearsal_manifest"

printf 'sema postgres repository and cutover rehearsal passed\n'
