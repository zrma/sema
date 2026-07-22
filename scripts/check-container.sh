#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
image="sema-container-check-$$:local"
container=
volume="sema-container-check-$$"

cleanup() {
  if [ -n "$container" ]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  docker volume rm -f "$volume" >/dev/null 2>&1 || true
  docker image rm -f "$image" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

command -v docker >/dev/null 2>&1 || {
  printf 'container check failed: docker is required\n' >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  printf 'container check failed: docker daemon is unavailable\n' >&2
  exit 1
}

docker compose -f deploy/compose.yaml config -q
docker build --pull=false --build-arg VERSION=v0.0.0-test -t "$image" . >/dev/null

[ "$(docker image inspect --format '{{.Config.User}}' "$image")" = "65532:65532" ] || {
  printf 'container check failed: image user is not the unprivileged runtime identity\n' >&2
  exit 1
}
docker run --rm "$image" -version | grep -Fxq 'sema-server v0.0.0-test' || {
  printf 'container check failed: embedded server version is incorrect\n' >&2
  exit 1
}
docker run --rm --entrypoint /usr/local/bin/sema-target-server "$image" -version | grep -Fxq 'sema-target-server v0.0.0-test' || {
  printf 'container check failed: embedded target server version is incorrect\n' >&2
  exit 1
}
docker run --rm --entrypoint /usr/local/bin/sema-target-smoke "$image" -version | grep -Fxq 'sema-target-smoke v0.0.0-test' || {
  printf 'container check failed: embedded target smoke version is incorrect\n' >&2
  exit 1
}
docker run --rm --entrypoint /usr/local/bin/sema-postgres-migrate "$image" -version | grep -Fxq 'sema-postgres-migrate v0.0.0-test' || {
  printf 'container check failed: embedded migration runner version is incorrect\n' >&2
  exit 1
}
docker run --rm --entrypoint /usr/local/bin/sema-healthcheck "$image" -version | grep -Fxq 'sema-healthcheck v0.0.0-test' || {
  printf 'container check failed: embedded healthcheck version is incorrect\n' >&2
  exit 1
}
docker run --rm --entrypoint /usr/local/bin/sema-ops-check "$image" \
  -cycles 1 -tickets-per-cycle 20 -concurrency 4 -timeout 30s >/dev/null

docker volume create "$volume" >/dev/null
container=$(docker run -d \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --mount "type=volume,source=$volume,target=/var/lib/sema" \
  "$image" \
  -listen 0.0.0.0:8080 \
  -journal /var/lib/sema/sema.journal \
  -reservation-ttl 30s \
  -allow-unauthenticated-remote)

attempt=0
until docker exec "$container" /usr/local/bin/sema-healthcheck >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    printf 'container check failed: service did not become ready\n' >&2
    exit 1
  fi
  sleep 1
done

docker stop -t 15 "$container" >/dev/null
docker start "$container" >/dev/null
attempt=0
until docker exec "$container" /usr/local/bin/sema-healthcheck >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    printf 'container check failed: service did not recover after restart\n' >&2
    exit 1
  fi
  sleep 1
done

printf 'sema container check passed\n'
