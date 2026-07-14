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
  docs/REPO_MANIFEST.yaml \
  docs/todo-0001-foundation/spec.md \
  docs/todo-0001-foundation/open-questions.md; do
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

printf 'sema repository checks passed\n'
