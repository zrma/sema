# Product Roadmap

## P0: Architecture Foundation

- [x] Sema repository와 `jj` Git backend 초기화
- [x] GPT-5.6 AI-first agent harness와 publication boundary gate 적용
- [x] 초기 domain vocabulary와 component boundary 문서화
- [ ] canonical entity schema와 lifecycle 정의
- [ ] new-match와 backfill reference fixture 정의
- [ ] implementation stack decision record 작성

## P1: Deterministic Match Core

- [ ] immutable matchmaking snapshot
- [ ] candidate partition과 bounded enumeration
- [ ] hard constraint evaluation
- [ ] soft objective scoring과 explanation
- [ ] deterministic proposal generation과 replay test

## P2: Reservation And Assignment

- [ ] proposal acceptance와 rejection lifecycle
- [ ] idempotent reservation
- [ ] conflict detection과 retry policy
- [ ] assignment commit과 cancellation
- [ ] backfill roster update

## P3: Runtime And Operations

- [ ] ticket/session ingestion API
- [ ] persistence와 recovery
- [ ] horizontal worker coordination
- [ ] metrics, traces, decision audit
- [ ] load, soak, failure-injection validation

## P4: Policy And Ecosystem

- [ ] versioned policy contract
- [ ] rule simulation과 offline evaluation
- [ ] SDK와 integration examples
- [ ] compatibility and migration policy
- [ ] distribution and release workflow
