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
