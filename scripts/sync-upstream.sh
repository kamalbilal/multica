#!/usr/bin/env bash
# Sync this fork with multica-ai/multica (upstream).
# Usage: ./scripts/sync-upstream.sh [--push]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PUSH=false
if [[ "${1:-}" == "--push" ]]; then
  PUSH=true
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "Not a git repository: $ROOT" >&2
  exit 1
fi

if ! git remote | grep -qx upstream; then
  echo "Adding upstream remote..."
  git remote add upstream https://github.com/multica-ai/multica.git
fi

echo "Fetching upstream..."
git fetch upstream

BRANCH="$(git branch --show-current)"
if [[ "$BRANCH" != "main" ]]; then
  echo "Checking out main..."
  git checkout main
fi

BEHIND="$(git rev-list --count main..upstream/main 2>/dev/null || echo 0)"
if [[ "$BEHIND" == "0" ]]; then
  echo "Already up to date with upstream/main."
  exit 0
fi

echo "Merging upstream/main ($BEHIND commit(s) behind)..."
git merge upstream/main --no-edit

if $PUSH; then
  echo "Pushing to origin..."
  git push origin main
fi

echo "Done. See FORK.md for fork-specific workflow."
