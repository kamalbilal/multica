#!/usr/bin/env bash
# Production build for `just build` — no GNU make required (Windows Git Bash).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${1:-.env}"

if [ ! -f "$REPO_ROOT/$ENV_FILE" ]; then
  printf 'missing env file: %s\n' "$ENV_FILE" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$REPO_ROOT/$ENV_FILE"
set +a
# shellcheck disable=SC1091
. "$REPO_ROOT/scripts/local-env.sh"

COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || printf unknown)"
DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
VERSION="${VERSION:-dev}"

EXE=""
case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*) EXE=.exe ;;
esac

printf '==> Building Go binaries (commit %s)\n' "$COMMIT"
(
  cd "$REPO_ROOT/server"
  go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}" -o "bin/server${EXE}" ./cmd/server
  go build -o "bin/migrate${EXE}" ./cmd/migrate
  if ! go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -o "bin/multica${EXE}" ./cmd/multica; then
    printf '    multica CLI is in use — run: just down\n' >&2
    exit 1
  fi
)

if [ ! -d "$REPO_ROOT/node_modules" ]; then
  printf '==> Installing frontend dependencies\n'
  (cd "$REPO_ROOT" && pnpm install)
fi

printf '==> Cleaning previous Next.js build\n'
rm -rf "$REPO_ROOT/apps/web/.next"

printf '==> Building Next.js web app\n'
(
  cd "$REPO_ROOT"
  pnpm --filter @multica/web build
)

if [ ! -f "$REPO_ROOT/apps/web/.next/BUILD_ID" ] \
  || [ ! -d "$REPO_ROOT/apps/web/.next/static" ]; then
  printf 'Next.js build looks incomplete. Re-run: just build\n' >&2
  exit 1
fi

if [ -f "$REPO_ROOT/scripts/ensure-cursor-sdk-executor.sh" ]; then
  printf '==> Building cursor_sdk executor\n'
  bash "$REPO_ROOT/scripts/ensure-cursor-sdk-executor.sh" >/dev/null || true
fi

printf '✓ Production build ready. Run: just up\n'
