#!/usr/bin/env bash
# Run api/web/desktop without GNU make. MODE is "dev" (hot reload) or "production"
# (compiled server + next start).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPONENT="${1:?component required (api|web|desktop)}"
ENV_FILE="${2:-.env}"
COMMIT="${3:-unknown}"
MODE="${4:-dev}"

EXE=""
case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*) EXE=.exe ;;
esac

set -a
# shellcheck disable=SC1090
. "$REPO_ROOT/$ENV_FILE"
set +a
# shellcheck disable=SC1091
. "$REPO_ROOT/scripts/local-env.sh"

case "$MODE" in
  dev|production) ;;
  *) printf 'unknown mode: %s (expected dev or production)\n' "$MODE" >&2; exit 1 ;;
esac

case "$COMPONENT" in
  api)
    if [ "$MODE" = production ]; then
      [ -f "$REPO_ROOT/server/bin/server$EXE" ] \
        || { printf 'missing server binary — run: just build\n' >&2; exit 1; }
      cd "$REPO_ROOT/server"
      exec "./bin/server$EXE"
    fi
    cd "$REPO_ROOT/server"
    exec go run -ldflags "-X main.commit=${COMMIT}" ./cmd/server
    ;;
  web)
    if [ "$MODE" = production ]; then
      [ -f "$REPO_ROOT/apps/web/.next/BUILD_ID" ] \
        && [ -d "$REPO_ROOT/apps/web/.next/static" ] \
        || { printf 'missing or incomplete web build — run: just build\n' >&2; exit 1; }
      cd "$REPO_ROOT/apps/web"
      export PORT="${FRONTEND_PORT:-3000}"
      exec pnpm start
    fi
    cd "$REPO_ROOT"
    exec pnpm dev:web
    ;;
  desktop)
    cd "$REPO_ROOT"
    exec pnpm dev:desktop
    ;;
  *)
    printf 'unknown component: %s\n' "$COMPONENT" >&2
    exit 1
    ;;
esac
