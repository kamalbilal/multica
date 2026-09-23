#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- Check prerequisites ----------
missing=()
command -v node >/dev/null 2>&1 || missing+=("node")
command -v pnpm >/dev/null 2>&1 || missing+=("pnpm")
command -v go >/dev/null 2>&1 || missing+=("go")
command -v docker >/dev/null 2>&1 || missing+=("docker")

if [ ${#missing[@]} -gt 0 ]; then
  echo "✗ Missing prerequisites: ${missing[*]}"
  echo "  Please install: Node.js 22, pnpm 10.28.2, Go 1.26.6, Docker"
  exit 1
fi

# ---------- Environment file ----------
if [ -f .git ]; then
  # Inside a git worktree (.git is a file, not a directory)
  ENV_FILE=".env.worktree"
  if [ ! -f "$ENV_FILE" ]; then
    echo "==> Worktree detected. Generating $ENV_FILE..."
    bash scripts/init-worktree-env.sh "$ENV_FILE"
  fi
else
  ENV_FILE=".env"
  if [ ! -f "$ENV_FILE" ]; then
    echo "==> Creating $ENV_FILE from .env.example..."
    cp .env.example "$ENV_FILE"
  fi
fi

echo "==> Using $ENV_FILE"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

# shellcheck disable=SC1091
. scripts/local-env.sh

# ---------- Install dependencies ----------
if [ ! -d node_modules ]; then
  echo "==> Installing dependencies..."
  pnpm install
fi

# ---------- Database ----------
bash scripts/ensure-postgres.sh "$ENV_FILE"

echo "==> Running migrations..."
(cd server && go run ./cmd/migrate up)

case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*)
    executor_path="$(powershell -NoProfile -ExecutionPolicy Bypass -File scripts/ensure-cursor-sdk-executor.ps1 2>/dev/null | tr -d '\r' || true)"
    ;;
  *)
    executor_path="$(bash scripts/ensure-cursor-sdk-executor.sh 2>/dev/null || true)"
    ;;
esac
if [ -n "$executor_path" ] && [ -f "$executor_path" ]; then
  export MULTICA_CURSOR_SDK_EXECUTOR="$executor_path"
  if ! grep -q '^MULTICA_CURSOR_SDK_EXECUTOR=' "$ENV_FILE" 2>/dev/null; then
    {
      printf '\n# Cursor SDK provider (fork) — see fork/docs/cursor-sdk-selfhost.md\n'
      printf 'MULTICA_CURSOR_SDK_EXECUTOR=%s\n' "$executor_path"
    } >> "$ENV_FILE"
  fi
fi

# ---------- Start services ----------
echo ""
echo "✓ Ready. Starting services..."
echo "  Backend:  http://localhost:${PORT:-8080}"
echo "  Frontend: http://localhost:${FRONTEND_PORT:-3000}"
echo ""

trap 'kill 0' EXIT
(cd server && go run ./cmd/server) &
pnpm dev:web &
wait
