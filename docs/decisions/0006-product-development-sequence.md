# ADR 0006: Engine-First Product Development Sequence

- Status: Accepted

## Context

deterministic planner, in-process lifecycle와 offline simulation은 있지만 실제 production consumer, numeric SLO와 stable external API contract는 아직 없다. 이 상태에서 transport, persistence와 distributed coordination을 먼저 고정하면 증거 없이 운영 복잡도와 compatibility surface가 커진다.

## Decision

Sema는 우선 policy-driven deterministic match composition engine과 evaluation lab으로 발전시킨다. 장기 milestone은 다음 순서를 따른다.

1. P5에서 executable evaluation lab과 built-in reference corpus를 제공한다.
2. P6에서 현실적인 workload model, quality metric과 small-case oracle comparison을 만든다.
3. P7에서 candidate index/partition과 대규모 queue의 성능·근사 품질을 검증한다.
4. P8에서 실제 consumer evidence를 바탕으로 public `v0alpha` API, compatibility와 distribution을 결정한다.
5. P9에서 필요가 확인된 ingestion, persistence, recovery와 horizontal coordination을 구현한다.
6. P10에서 telemetry, decision audit, load/soak/failure-injection과 deployment gate를 완성한다.

각 milestone은 이전 단계의 executable evidence와 실제 revisit trigger를 입력으로 사용하며 미래 단계의 architecture를 미리 안정화하지 않는다.

## Consequences

- `sema-lab`은 public executable이지만 Go package와 JSON schema는 아직 stable compatibility를 약속하지 않는다.
- P6와 P7이 API/server보다 먼저 quality와 scalability evidence를 만든다.
- database, transport, multi-replica authority와 numeric SLO는 P9/P10의 실제 consumer·deployment input이 생길 때 결정한다.
- release automation은 importable API 또는 distributable executable의 compatibility boundary가 정해지는 P8에서 완성한다.

## Revisit Triggers

- 별도 process consumer가 즉시 필요한 integration commitment가 생긴다.
- producer replay가 불가능하거나 durable assignment audit이 선행 요구사항이 된다.
- 현재 core가 측정된 production SLO를 충족하지 못한다.
- 외부 프로젝트가 stable Go API 또는 versioned wire schema를 요구한다.
