#!/usr/bin/env bash
# Build the cursor_sdk Node executor and print its absolute path on stdout.
# Exits 0 even when skipped (no node or node < 22) so dev bootstrap can continue.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EXECUTOR_DIR="$REPO_ROOT/fork/packages/cursor-sdk-executor"
EXECUTOR_SCRIPT="$EXECUTOR_DIR/dist/cli.js"

if [ ! -f "$EXECUTOR_DIR/package.json" ]; then
  exit 0
fi

if ! command -v node >/dev/null 2>&1; then
  echo "cursor_sdk: node not on PATH; skipping executor build" >&2
  exit 0
fi

node_major="$(node -p "Number(process.versions.node.split('.')[0])")"
if [ "$node_major" -lt 22 ]; then
  echo "cursor_sdk: node $node_major < 22; skipping executor build" >&2
  exit 0
fi

verify_cursor_sdk_executor_deps() {
  if [ ! -f "$EXECUTOR_DIR/node_modules/@cursor/sdk/package.json" ]; then
    return 1
  fi
  (cd "$EXECUTOR_DIR" && node --input-type=module -e "import '@cursor/sdk'" >/dev/null 2>&1)
}

echo "==> Building cursor_sdk executor..." >&2
(
  cd "$EXECUTOR_DIR"
  npm ci --silent
  if ! verify_cursor_sdk_executor_deps; then
    echo "cursor_sdk: @cursor/sdk install incomplete; retrying npm ci..." >&2
    rm -rf node_modules
    npm ci --silent
    verify_cursor_sdk_executor_deps || {
      echo "cursor_sdk executor dependencies are broken after npm ci (missing @cursor/sdk). Run 'npm ci' in $EXECUTOR_DIR and retry." >&2
      exit 1
    }
  fi
  npm run build --silent
)
printf '%s\n' "$EXECUTOR_SCRIPT"
