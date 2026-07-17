#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

for required_file in \
  LICENSE \
  README.md \
  AGENTS.md \
  Dockerfile \
  .dockerignore \
  .github/workflows/release.yml \
  alpha/compose.go \
  alpha/types.go \
  cmd/sema-server/main.go \
  cmd/sema-healthcheck/main.go \
  cmd/sema-benchmark-gate/main.go \
  cmd/sema-ops-check/main.go \
  cmd/sema-tui/main.go \
  cmd/sema-flow-report/main.go \
  cmd/sema-flow-matrix/main.go \
  deploy/compose.yaml \
  examples/compose/main.go \
  internal/api/v0alpha1/types.go \
  internal/httpapi/handler.go \
  internal/observability/recorder.go \
  internal/operational/load.go \
  internal/performance/report.go \
  internal/flow/simulator.go \
  internal/flowmatrix/matrix.go \
  internal/flowui/model.go \
  internal/league/population.go \
  docs/agent-harness.md \
  docs/HANDOFF.md \
  docs/status.md \
  docs/roadmap.md \
  docs/architecture.md \
  docs/domain-model.md \
  docs/lifecycle.md \
  docs/reference-scenarios.md \
  docs/reference-workloads.md \
  docs/sema-lab.md \
  docs/workload-evaluation.md \
  docs/evaluation-baseline.md \
  docs/candidate-discovery.md \
  docs/public-api.md \
  docs/api-compatibility.md \
  docs/releasing.md \
  docs/durable-runtime.md \
  docs/service-api.md \
  docs/observability.md \
  docs/operational-validation.md \
  docs/operations-runbook.md \
  docs/performance-slo.md \
  docs/release-admission.md \
  docs/sema-flow.md \
  docs/sema-flow-measurement.md \
  docs/sema-flow-capacity-matrix.md \
  docs/policy-simulation.md \
  docs/runtime-validation.md \
  docs/decisions/0001-implementation-baseline.md \
  docs/decisions/0002-runtime-adapter-baseline.md \
  docs/decisions/0003-policy-identity.md \
  docs/decisions/0004-policy-catalog.md \
  docs/decisions/0005-integration-publication-baseline.md \
  docs/decisions/0006-product-development-sequence.md \
  docs/decisions/0007-evaluation-calibration-baseline.md \
  docs/decisions/0008-candidate-window-baseline.md \
  docs/decisions/0009-alpha-integration-release-baseline.md \
  docs/decisions/0010-durable-journal-baseline.md \
  docs/decisions/0011-http-service-baseline.md \
  docs/decisions/0012-observability-redaction-baseline.md \
  docs/decisions/0013-operational-validation-baseline.md \
  docs/decisions/0014-container-deployment-baseline.md \
  docs/decisions/0015-performance-release-gate.md \
  docs/migrations/v0alpha1-to-v0alpha2.md \
  docs/REPO_MANIFEST.yaml \
  docs/todo-0001-foundation/spec.md \
  docs/todo-0001-foundation/decisions.md \
  docs/todo-0002-objective-policy/spec.md \
  docs/todo-0002-objective-policy/policy.md \
  docs/todo-0003-assignment-lifecycle/spec.md \
  docs/todo-0004-application-runtime/spec.md \
  docs/todo-0005-runtime-adapter/spec.md \
  docs/todo-0006-runtime-validation/spec.md \
  docs/todo-0007-demand-index/spec.md \
  docs/todo-0008-policy-identity/spec.md \
  docs/todo-0009-policy-catalog/spec.md \
  docs/todo-0010-policy-simulation/spec.md \
  docs/todo-0011-integration-decision/spec.md \
  docs/todo-0012-sema-lab/spec.md \
  docs/todo-0013-workload-evaluation/spec.md \
  docs/todo-0014-candidate-discovery/spec.md \
  docs/todo-0015-public-integration/spec.md \
  docs/todo-0016-durable-runtime/spec.md \
  docs/todo-0017-http-service/spec.md \
  docs/todo-0018-observability/spec.md \
  docs/todo-0019-operational-validation/spec.md \
  docs/todo-0020-container-operations/spec.md \
  docs/todo-0021-performance-release-gate/spec.md \
  docs/todo-0022-sema-flow/spec.md \
  docs/todo-0023-population-simulation/spec.md \
  docs/todo-0024-flow-measurement/spec.md \
  docs/todo-0025-discrete-event-scheduler/spec.md \
  docs/todo-0026-capacity-matrix/spec.md \
  docs/todo-0027-unbounded-game-simulation/spec.md \
  docs/todo-0028-flow-trend-panels/spec.md \
  docs/todo-0029-global-proposal-batch-optimization/spec.md \
  docs/todo-0030-flow-batch-admission/spec.md \
  docs/todo-0031-single-select-performance/spec.md \
  docs/todo-0032-flow-lifecycle-entry-motion/spec.md \
  scripts/build-release.sh \
  scripts/check-container.sh \
  scripts/check-performance.sh \
  scripts/check-release-admission.sh \
  scripts/check-release-build.sh \
  go.mod \
  go.sum; do
  [ -s "$required_file" ] || {
    printf 'repository check failed: missing or empty %s\n' "$required_file" >&2
    exit 1
  }
done

grep -Fxq 'module github.com/zrma/sema' go.mod || {
  printf 'repository check failed: canonical Go module identity is missing\n' >&2
  exit 1
}

grep -Fxq 'go 1.26.0' go.mod && grep -Fq '"go_version":"1.26.0"' scripts/check-performance.sh || {
  printf 'repository check failed: target profile Go version must match go.mod\n' >&2
  exit 1
}

grep -Fq 'Apache License' LICENSE || {
  printf 'repository check failed: Apache-2.0 license text is missing\n' >&2
  exit 1
}

grep -Fxq '  class: public' docs/REPO_MANIFEST.yaml || {
  printf 'repository check failed: manifest publication class must be public\n' >&2
  exit 1
}

scripts/check-agent-harness-interface.sh
scripts/check-publication-boundary.py --self-test

grep -Fq '# Created by https://www.toptal.com/developers/gitignore/api/' .gitignore || {
  printf 'repository check failed: .gitignore is not sourced from gitignore.io\n' >&2
  exit 1
}

git check-ignore -q .env || {
  printf 'repository check failed: local environment files are not ignored\n' >&2
  exit 1
}

if grep -R -n -F '"github.com/zrma/sema/internal/' examples; then
  printf 'repository check failed: public examples must not import internal packages\n' >&2
  exit 1
fi

sh -n scripts/build-release.sh scripts/check-container.sh scripts/check-performance.sh scripts/check-release-admission.sh scripts/check-release-build.sh

grep -Eq '^FROM golang:[^ ]+@sha256:[0-9a-f]{64} AS build$' Dockerfile || {
  printf 'repository check failed: container builder must use an exact digest\n' >&2
  exit 1
}

grep -Fq '127.0.0.1:8080:8080' deploy/compose.yaml || {
  printf 'repository check failed: unauthenticated deployment must bind host loopback\n' >&2
  exit 1
}

unformatted=$(find . -type f -name '*.go' -not -path './vendor/*' -exec gofmt -l {} +)
if [ -n "$unformatted" ]; then
  printf 'repository check failed: unformatted Go files\n%s\n' "$unformatted" >&2
  exit 1
fi

go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
go run ./cmd/sema-lab -list >/dev/null
go run ./cmd/sema-lab team-2v2-mixed >/dev/null
go run ./cmd/sema-lab -format json battle-royale-duo >/dev/null
go run ./cmd/sema-lab -format json diagnostic-bounded-quality-gap diagnostic-candidate-window-gap synthetic-5v5-seeded-queue >/dev/null
go run ./examples/compose >/dev/null
go run ./cmd/sema-server -version >/dev/null
go run ./cmd/sema-healthcheck -version >/dev/null
go run ./cmd/sema-benchmark-gate -version >/dev/null
go run ./cmd/sema-ops-check -cycles 1 -tickets-per-cycle 20 -concurrency 4 -timeout 30s >/dev/null
go run ./cmd/sema-tui -version >/dev/null
go run ./cmd/sema-tui -snapshot -population 40 -game-duration 20s -steps 80 -width 100 -height 32 >/dev/null
go run ./cmd/sema-flow-report -version >/dev/null
go run ./cmd/sema-flow-report -duration 60s -population 40 -game-duration 20s -max-return-delay 10s -format json >/dev/null
go run ./cmd/sema-flow-matrix -duration 3s -population 40 -seeds 42,43 -batches 1,2 -parallel 2 -game-duration 20s -arrival-interval 100ms -planning-interval 1s -max-return-delay 10s -format json >/dev/null
scripts/check-release-build.sh
go test ./internal/planner -run '^$' -bench '^BenchmarkPlan' -benchtime=1x
go test ./internal/engine -run '^$' -bench '^BenchmarkEngine' -benchtime=1x
go test ./internal/durable -run '^$' -bench '^BenchmarkOpenReplay$' -benchtime=1x

printf 'sema repository checks passed\n'
