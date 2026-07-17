# Sema Agent Guide

이 파일은 짧은 bootstrap map이다. 공통 실행 계약과 Sema의 domain-specific 규칙은 repository-owned 문서가 소유한다.

## First Read

- 공통 하네스 인터페이스와 Sema overlay: `docs/agent-harness.md`.
- 현재 상태와 방향: `docs/HANDOFF.md`, `docs/status.md`, `docs/roadmap.md`.
- architecture, public API, 현재 작업: `docs/architecture.md`, `docs/public-api.md`, 활성 `docs/todo-*/spec.md`.

<!-- agent-harness-baseline:start -->
## Agent Harness Baseline (GPT-5.6)

Baseline ID: `openai-gpt-5.6-2026-07-11`.

- Source of truth: use the `openai-docs` skill and the official [latest model guide](https://developers.openai.com/api/docs/guides/latest-model) plus [prompting best practices](https://developers.openai.com/api/docs/guides/latest-model#prompting-best-practices) before changing OpenAI model, API, prompt, or agent guidance.
- Model target: when the task asks for the current or latest OpenAI baseline, use `gpt-5.6`. This is harness guidance, not proof that the application calls OpenAI; change runtime model strings only at an existing OpenAI integration point.
- Prompt budget: start with the smallest prompt and task-relevant tool set that reliably completes the work. Preserve project-specific constraints, remove redundant generic instructions, and add examples only for an observed failure.
- Request modes: for answer, explain, review, diagnose, or plan requests, inspect and report without implementation. For change, build, or fix requests, make the requested in-scope local changes and run relevant non-destructive validation.
- Permissions: reading, searching, editing in-scope files, and running non-destructive checks are pre-authorized for change tasks. Require confirmation for external writes not explicitly requested, destructive or irreversible actions, purchases or cost, secrets, or material scope expansion.
- Persistence: continue until the requested outcome is complete; do not stop after only analysis, a partial patch, or an intermediate tool success. Stop and escalate only at a real permission, product-decision, or external-state boundary.
- Verification: treat tool and patch success as provisional. Re-read the diff and verify the user-visible or runtime outcome with the narrowest meaningful checks, then broaden only when risk warrants it.
- Publication boundary: before a public push, tag/release, visibility change, or published-history rewrite, run the repository boundary check and any authorized local private-inventory check. Keep private inventory outside published repositories and CI configuration; retain only non-identifying responsibility boundaries and operational contracts.
- Tracked-artifact privacy: treat tool output, memory-derived environment context, local absolute paths, machine/host/cluster identifiers, internal endpoints or addresses, and full diagnostic logs as local-only by default. Do not paste raw stdout or stderr into tracked files; retain repository-owned decisions and redacted verification outcomes with placeholders such as `<repo-root>`, `<private-host>`, `<internal-ip>`, and `<cluster-context>`.
- Output: lead with the conclusion. Include required evidence, material caveats, and the next action; trim introductions, repetition, generic reassurance, and optional background before trimming required content.
- Structure: use a lightweight task-specific plan or output shape. Do not impose a global template or long process narration when the repository already supplies the necessary workflow.
- Modes and orchestration: configure Pro mode in the API or runtime rather than asking the model to “think harder.” Use Programmatic Tool Calling only for bounded reduction stages with explicit schemas, limits, and no approval-sensitive side effects; keep semantic decisions and final validation direct.
- Evaluation: add or retain harness instructions only when repository checks or representative tasks show they improve final-answer completeness, evidence quality, reliability, latency, or cost. Evaluate the final result, not just tool-call count.
- Project overlay: the remaining sections of this file and the linked project docs define domain-specific architecture, tests, safety boundaries, escalation rules, and publish gates. They may specialize this baseline but must not silently weaken its permission or evidence requirements.
<!-- agent-harness-baseline:end -->

## Project Overlay

- domain contract와 state transition은 `docs/architecture.md` 및 활성 foundation spec을 source of truth로 사용한다.
- 새 매치와 backfill을 같은 탐색 core에서 다루되 입력 타입과 lifecycle은 명시적으로 구분한다.
- proposal 생성과 reservation/commit side effect를 분리하고 deterministic replay evidence를 우선한다.
- 구현 baseline은 ADR 0001의 Go 단일 프로세스·인메모리 구조를 따르며 변경은 새 architecture decision으로 남긴다.
- 기본 검증은 `scripts/check.sh`; 로컬 VCS는 `jj`; push는 명시적 권한이 있을 때만 수행한다.
- 장기 milestone 순서는 ADR 0006을 따르며 현재 executable evidence는 `cmd/sema-lab`에서 확인한다.
- P6 quality 비교는 `docs/workload-evaluation.md`의 coverage/fairness metric과 bounded oracle 한계를 지킨다.
- P7 candidate window는 opt-in approximation이며 discovery truncation과 quality gap evidence를 숨기지 않는다.
- public surface는 `alpha.Compose`의 side-effect-free composition으로 제한하고 internal type alias나 coordinator lifecycle을 노출하지 않는다.
- release automation은 publication 승인과 local private-inventory gate를 대체하지 않는다.
- P9 durable mutation은 `docs/durable-runtime.md`의 sync-before-success, fixed TTL과 single-writer replay contract를 지킨다.
- HTTP lifecycle은 `docs/service-api.md`의 server clock, proposal ID authority와 loopback-default boundary를 지킨다.
- metrics/traces/audit에는 resource ID나 raw durable payload를 넣지 않고 `docs/observability.md`의 cardinality/redaction 경계를 지킨다.
- operational gate는 외부 service를 변경하지 않는 격리 runtime에서 실행하고 `docs/operational-validation.md`의 workload/recovery 계약을 지킨다.
- container deployment는 `docs/operations-runbook.md`의 non-root, host-loopback, replica 1과 offline backup 경계를 약화하지 않는다.
- performance evidence는 raw CPU/host output을 추적하지 않고 `docs/performance-slo.md`의 sanitized aggregate와 reference budget만 보존한다.
- v1/stable publication은 `docs/release-admission.md`의 blocker와 machine-readable admission flag가 모두 해소되기 전에는 시도하지 않는다.
- Flow의 hidden true skill은 synthetic game result에만 사용하고 planner에는 visible rating만 전달한다.
- closed population, outcome curve와 Elo update는 `docs/sema-flow.md`의 reference simulation이며 production MMR이나 scheduler contract로 승격하지 않는다.
- Flow measurement는 `docs/sema-flow-measurement.md`의 player-weighted wait, time-weighted saturation과 fixed-point throughput 계약을 따르며 제품 SLA로 해석하지 않는다.
- Flow simulation event frame은 logical time을 소유하지 않는다. `docs/todo-0025-discrete-event-scheduler/spec.md`의 due ingress, stable timestamp ordering과 horizon backlog 계약을 유지한다.
- Flow capacity 비교는 `docs/sema-flow-capacity-matrix.md`의 동일-demand gate와 min/median/max contract를 지키며 product target 없이 profile 권장이나 production capacity를 선언하지 않는다.
- Sema의 Flow 책임은 assignment confirm까지다. active game 수는 planning eligibility를 제한하지 않으며 frontend-owned game/result/return 흐름을 보여주는 TUI `MATCH LIFECYCLE` 패널은 관찰 surface로 유지한다.
- Flow trend panel은 같은 logical timestamp를 합치고 bounded history를 유지한다. queue wait는 pre-confirm ticket을 player-weighted하고 rating density는 measurement schema를 바꾸지 않는 1500-centered TUI read model을 사용한다.
- Flow rating density의 vertical scaling은 기존 9개 centered histogram bucket을 반복 렌더링하는 presentation-only 확대다. 반복 row를 추가 rating sample이나 더 정밀한 measurement로 해석하지 않는다.
- Flow queue departure의 selected hold, match marker/color, horizontal travel과 vertical compaction은 presentation-only다. active lifecycle visual slot은 match completion 전 재사용하지 않고 waiting과 lifecycle block 전체가 같은 accent를 유지한다. reduced-motion은 최종 layout을 즉시 적용하며 motion frame이 planner, reservation, confirmation이나 logical clock을 지연시키면 안 된다.
- Flow lifecycle entry는 batch proposal 순서대로 panel 상단에서 stagger되고 기존 block을 아래로 이동시키는 presentation-only motion이다. entry 중에도 최신 reservation/confirmation stage를 렌더링하며 reduced-motion과 snapshot은 최종 layout을 즉시 적용한다.
- P18 planner는 greedy cover와 anchored search로 admissible candidate graph를 만들고, backfill-first weighted set-packing으로 `MaxProposals` 이하의 total rank utility를 최적화한다. generation/selection budget과 truncation evidence를 숨기거나 rank utility를 실행 간 절대 quality로 해석하지 않는다.
- P23 batch frontier는 최대 12 match ticket, 2 backfill ticket과 2 team의 evaluation-only exhaustive evidence다. coverage/wait와 per-match quality의 Pareto relation을 production SLA, calibrated utility 또는 planner execution path로 승격하지 않는다.
- P24 default small-queue expanded path는 candidate budget이 명시되지 않은 P23 boundary에서만 distinct ticket-set alternative와 Pareto repair를 사용한다. explicit approximation, large queue와 single-select fast path의 budget/evidence를 약화하지 않는다.
- P25 wait-priority service는 backfill-count tier 안에서 oldest eligible demand를 rank utility보다 먼저 선택한다. explicit window/candidate/node truncation은 fairness guarantee 밖이며 eligible/selected priority evidence와 `BudgetExhausted`를 숨기지 않는다.
- P26 backfill quality context는 ticket revision과 `rosterVersion`에 묶인 aggregate만 사용한다. full roster identity를 public input에 복제하지 않고 resulting skill/role/latency evidence와 stale reserve contract를 함께 검증한다.
- P27 discovery index는 linear oldest-fitting window와 exact-equivalent해야 한다. stateless plan에 one-time build cost를 숨기지 않고 incremental lifetime은 stateful demand repository/productization boundary가 소유한다.
- matcher 장기 순서와 V0 service prototype 이후 persistence/API productization 진입 기준은 `docs/matcher-v0-exit.md`를 따른다.
- Flow의 `matches_per_cycle`은 fixed fill target이 아니라 proposal upper bound다. 5v5 한 match 분량부터 planning하고 기본 32-match burst를 허용하되 synthetic 처리량을 production capacity나 SLA로 승격하지 않는다.
- selector가 둘 이상의 proposal을 선택할 수 없으면 redundant anchored candidate graph를 만들지 않는다. single-select fast path를 바꿀 때는 P20 reference benchmark와 multi-proposal diversity fixture를 함께 검증한다.
