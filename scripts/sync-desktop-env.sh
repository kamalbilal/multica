#!/usr/bin/env bash
# Point the desktop Canary build at this checkout's backend (from root .env).
# Used by `just up` and scripts/desktop-dev-launch.bat before preview:desktop.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${1:-.env}"
DESKTOP_ENV="$REPO_ROOT/apps/desktop/.env.development.local"

if [ ! -f "$REPO_ROOT/$ENV_FILE" ]; then
  printf 'missing env file: %s\n' "$ENV_FILE" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$REPO_ROOT/$ENV_FILE"
set +a

backend="${PORT:-8080}"
frontend="${FRONTEND_PORT:-3000}"

cat > "$DESKTOP_ENV" <<EOF
# Managed by scripts/sync-desktop-env.sh from ${ENV_FILE}.
VITE_API_URL=http://localhost:${backend}
VITE_WS_URL=ws://localhost:${backend}/ws
VITE_APP_URL=http://localhost:${frontend}
EOF

printf 'Desktop env → API http://localhost:%s (rebuild desktop if it was already running)\n' "$backend"

if command -v curl >/dev/null 2>&1; then
  if ! curl -sf --max-time 3 "http://localhost:${backend}/health" >/dev/null 2>&1; then
    printf 'WARN: nothing healthy on http://localhost:%s — run `just up` before signing in.\n' "$backend" >&2
  fi
fi
