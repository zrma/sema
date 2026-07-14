#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

for required_file in \
  README.md \
  AGENTS.md \
  docs/agent-harness.md \
  docs/HANDOFF.md \
  docs/status.md \
  docs/roadmap.md \
  docs/architecture.md \
  docs/domain-model.md \
  docs/lifecycle.md \
  docs/reference-scenarios.md \
  docs/reference-workloads.md \
  docs/decisions/0001-implementation-baseline.md \
  docs/REPO_MANIFEST.yaml \
  docs/todo-0001-foundation/spec.md \
  docs/todo-0001-foundation/decisions.md \
  docs/todo-0002-objective-policy/spec.md \
  docs/todo-0002-objective-policy/policy.md \
  docs/todo-0003-assignment-lifecycle/spec.md \
  docs/todo-0004-application-runtime/spec.md \
  go.mod; do
  [ -s "$required_file" ] || {
    printf 'repository check failed: missing or empty %s\n' "$required_file" >&2
    exit 1
  }
done

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

unformatted=$(find . -type f -name '*.go' -not -path './vendor/*' -exec gofmt -l {} +)
if [ -n "$unformatted" ]; then
  printf 'repository check failed: unformatted Go files\n%s\n' "$unformatted" >&2
  exit 1
fi

go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
go test ./internal/planner -run '^$' -bench '^BenchmarkPlan' -benchtime=1x

printf 'sema repository checks passed\n'
