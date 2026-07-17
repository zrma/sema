# ADR 0013: Isolated Service Load And Recovery Validation

- Status: Accepted

## Context

unit benchmark와 durable replay fixture는 개별 component를 검증하지만 concurrent HTTP ingestion부터 terminal acknowledgment, observability scrape와 restart recovery가 함께 유지되는지는 보여주지 않는다. production endpoint를 부하 도구의 기본 대상으로 삼으면 권한과 데이터 경계를 넓히고 재현 가능한 repository gate도 잃는다.

## Decision

- repository-owned operational validator는 loopback HTTP server와 새 임시 single-writer journal을 직접 소유한다.
- fixed 2x5 solo workload를 여러 cycle 실행하고 모든 proposal을 reserve, confirm, complete한다.
- quick gate는 20 ticket 한 cycle이며 manual soak는 같은 workload의 cycle 수만 늘린다.
- lifecycle 뒤 process restart를 모사해 terminal assignment와 audit prefix를 replay한다.
- newline 없는 final record를 append해 interrupted disk write를 모사하고 complete prefix recovery를 검증한다.
- report는 aggregate count, latency summary와 boolean recovery evidence만 반환하고 resource ID와 path를 제외한다.
- observed latency는 numeric product SLO가 아니라 다음 repeated target-profile gate의 입력이다.

## Consequences

- service, observability와 journal recovery의 regression을 한 command로 재현할 수 있다.
- command가 external endpoint나 persistent user data를 변경하지 않는다.
- 정상 종료 뒤의 restart와 incomplete tail은 검증하지만 sudden process kill, filesystem exhaustion와 storage-device durability를 완전히 모사하지 않는다.
- actual deployment, authentication, multi-replica와 target hardware SLO는 후속 gate가 소유한다.

## Revisit Triggers

- remote staging endpoint에 대한 승인된 load tool이 필요하다.
- workload에 backfill, mixed party 또는 arrival pacing이 필요하다.
- crash-consistent filesystem/container harness가 추가된다.
- target runtime profile과 numeric SLO가 확정된다.
