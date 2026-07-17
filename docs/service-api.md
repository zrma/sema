# Sema Service API v0alpha1

## Scope

`cmd/sema-server`는 `internal/durable.Runtime` 위에서 experimental JSON/HTTP API를 제공한다. ticket와 backfill session demand를 ingest하고, durable planning/reservation/assignment lifecycle을 별도 process consumer가 실행할 수 있게 한다.

이 schema는 `v0alpha1`이며 stable compatibility를 약속하지 않는다. Go `alpha` package와 목적은 다르지만 둘 다 composition semantics를 공유한다.

## Server

```sh
go run ./cmd/sema-server \
  -listen 127.0.0.1:8080 \
  -journal var/sema.journal \
  -reservation-ttl 30s
```

authentication과 TLS는 아직 포함하지 않으므로 기본 listener는 loopback이다. non-loopback address는 `-allow-unauthenticated-remote`를 명시해야만 열리며 이 flag는 production security 승인이 아니다.

## Envelope

모든 API response는 다음 envelope 중 하나다.

```json
{"api_version":"v0alpha1","data":{}}
```

```json
{"api_version":"v0alpha1","error":{"code":"InvalidInput","message":"...","retryable":false}}
```

request JSON은 unknown field를 거부하고 최대 1 MiB다. response는 `application/json`, `Cache-Control: no-store`와 `X-Content-Type-Options: nosniff`를 사용한다.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/v0alpha1/policies/{version}` | versioned policy register/idempotent retry |
| `GET` | `/v0alpha1/policies/{version}` | policy와 fingerprint 조회 |
| `PUT` | `/v0alpha1/match-tickets/{ticket_id}` | match ticket upsert |
| `DELETE` | `/v0alpha1/match-tickets/{ticket_id}?revision=...` | exact revision cancel |
| `PUT` | `/v0alpha1/backfill-tickets/{ticket_id}` | session backfill demand upsert |
| `DELETE` | `/v0alpha1/backfill-tickets/{ticket_id}?revision=...&roster_version=...` | exact backfill freshness cancel |
| `POST` | `/v0alpha1/plans` | durable multi-proposal planning decision |
| `POST` | `/v0alpha1/reservations/{reservation_id}` | authoritative `proposal_id` reserve |
| `POST` | `/v0alpha1/reservations/{reservation_id}/confirm` | assignment confirm |
| `POST` | `/v0alpha1/reservations/{reservation_id}/cancel` | active reservation cancel |
| `GET` | `/v0alpha1/assignments/{assignment_id}` | assignment polling/read model |
| `POST` | `/v0alpha1/assignments/{assignment_id}/acknowledgments` | terminal complete/cancel/fail acknowledgment |
| `GET` | `/v0alpha1/audit?after=...&limit=...` | redacted durable decision audit page |

`/livez`, `/readyz`와 `/metrics`는 operational endpoint다. error response는 `X-Sema-Error-Code` header에도 bounded code를 제공한다. 상세 contract는 `docs/observability.md`가 소유한다.

request/response DTO는 `internal/api/v0alpha1`이 소유하고 domain struct를 JSON으로 직접 노출하지 않는다. relaxation duration은 `after_wait_millis`로 표현한다.

backfill request의 optional `existing_teams`는 `player_count`, `skill_total`, `role_counts[]`, `max_latency_millis`를 가진다. 제공된 aggregate는 `roster_version`과 함께 freshness authority가 되며 resulting roster quality evidence에 사용된다. 생략하면 legacy vacancy-only evaluation이다.

plan batch evidence에는 bounded candidate graph의 wait-priority eligible/selected demand 수와 oldest eligible/selected wait가 포함된다. 이는 additive diagnostic field이며 service wire marker는 계속 `v0alpha1`이다. public Go `alpha` marker와 독립적이지만 plan selection은 같은 P25 oldest-first semantics를 사용한다.

## Time And Authority

client는 planning/reservation/assignment time을 보내지 않는다. server clock이 wait relaxation, reservation expiry, confirmation과 acknowledgment time의 authority다.

`snapshot_id`는 planning idempotency key다. 같은 ID와 policy retry는 server time이 달라도 synced 최초 batch를 반환하고, 다른 policy로 같은 ID를 쓰면 `IdempotencyConflict`다.

plan response의 proposal은 synced `plan_completed` record에 저장된다. reserve request는 다음처럼 `proposal_id`만 보낸다.

```json
{"proposal_id":"<proposal-id>"}
```

server는 durable proposal index에서 exact content를 찾아 reserve한다. client가 placement, evidence 또는 ticket list를 다시 제출할 수 없으므로 planner를 우회한 forged proposal은 authority가 되지 않는다.

## Delivery And Retry

- caller가 `snapshot_id`, `reservation_id`, `assignment_id`, acknowledgment `operation_id`를 생성한다.
- timeout 또는 connection loss 뒤 같은 ID와 같은 payload를 반복한다.
- sync 뒤 response가 유실되어도 restart replay가 기존 reservation/assignment 결과를 반환한다.
- confirm response를 잃으면 같은 confirm을 반복하거나 assignment ID로 poll한다.
- pending assignment는 `GET`으로 조회하고 외부 allocation/session 적용 뒤 terminal acknowledgment를 보낸다.
- `retryable: true`는 같은 payload의 immediate retry가 아니라 replan, new reservation 또는 service recovery가 필요할 수 있음을 뜻한다.

현재 delivery는 synchronous response와 polling이다. push delivery, outbox subscriber와 automatic retry worker는 제공하지 않는다.

## Failure Mapping

- malformed/invalid input: `400`.
- stale, reservation conflict와 state/idempotency conflict: `409`.
- expired reservation: `410`.
- missing policy/proposal/assignment: `404`.
- durable runtime I/O/recovery failure: `503` with generic public message.

## Verification

```sh
go test ./internal/httpapi ./cmd/sema-server
go test -race ./internal/httpapi ./cmd/sema-server
go run ./cmd/sema-server -version
```

integration fixture는 API lifecycle을 실행하고 plan 직후와 assignment confirm 직후에 runtime을 각각 다시 열어 proposal authority, polling과 acknowledgment retry를 검증한다.
