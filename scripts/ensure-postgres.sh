#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env}"
caller_postgres_db="${POSTGRES_DB:-}"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing env file: $ENV_FILE"
  echo "Create .env from .env.example, or run 'make worktree-env' and use .env.worktree."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

if [ -n "$caller_postgres_db" ]; then
  POSTGRES_DB="$caller_postgres_db"
fi
POSTGRES_DB="${POSTGRES_DB:-multica}"
POSTGRES_USER="${POSTGRES_USER:-multica}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-multica}"
DATABASE_URL="${DATABASE_URL:-}"

export PGPASSWORD="$POSTGRES_PASSWORD"

db_host=""
db_port="${POSTGRES_PORT:-5432}"
db_name="$POSTGRES_DB"

parse_database_url() {
  local rest authority hostport path port_part

  rest="${DATABASE_URL#*://}"
  rest="${rest%%\?*}"
  authority="${rest%%/*}"
  path="${rest#*/}"

  if [ "$authority" = "$rest" ]; then
    path=""
  fi

  hostport="${authority##*@}"

  if [[ "$hostport" == \[* ]]; then
    db_host="${hostport#\[}"
    db_host="${db_host%%]*}"
    port_part="${hostport#*\]}"
    if [[ "$port_part" == :* ]] && [ -n "${port_part#:}" ]; then
      db_port="${port_part#:}"
    fi
  else
    db_host="${hostport%%:*}"
    if [[ "$hostport" == *:* ]] && [ -n "${hostport##*:}" ]; then
      db_port="${hostport##*:}"
    fi
  fi

  if [ -n "$path" ]; then
    db_name="${path%%/*}"
  fi
}

if [ -n "$DATABASE_URL" ]; then
  parse_database_url
fi

is_local() {
  [ -z "$DATABASE_URL" ] || [ "$db_host" = "localhost" ] || [ "$db_host" = "127.0.0.1" ] || [ "$db_host" = "::1" ]
}

# Any container already publishing localhost:db_port (e.g. multica-fork-postgres-1 on
# 5433 while this checkout's compose project is "multica").
postgres_container_on_port() {
  docker ps --filter "publish=${db_port}" --format '{{.Names}}' 2>/dev/null | head -n 1
}

# Stopped containers still remember their host port. Restart those instead of
# `docker compose up`, which would create a second empty volume on the same port.
stopped_postgres_container_for_port() {
  local id name bindings
  while IFS= read -r id; do
    [ -n "$id" ] || continue
    bindings="$(docker inspect -f '{{json .HostConfig.PortBindings}}' "$id" 2>/dev/null || true)"
    case "$bindings" in
      *"\"HostPort\":\"${db_port}\""*|*"\"HostPort\": \"${db_port}\""*)
        name="$(docker inspect -f '{{.Name}}' "$id" 2>/dev/null | sed 's#^/##')"
        if [ -n "$name" ]; then
          printf '%s\n' "$name"
          return 0
        fi
        ;;
    esac
  done < <(docker ps -aq --filter "status=exited" --filter "ancestor=pgvector/pgvector:pg17" 2>/dev/null)
  return 1
}

exec_postgres() {
  local container=$1
  shift
  docker exec -i "$container" "$@"
}

if is_local; then
  # ---------- Local: use Docker ----------
  export POSTGRES_PORT="$db_port"
  echo "==> Ensuring fork PostgreSQL container is running on localhost:${db_port}..."

  postgres_container="$(postgres_container_on_port)"
  if [ -n "$postgres_container" ]; then
    echo "    Reusing existing container ${postgres_container} on port ${db_port}"
  elif stopped="$(stopped_postgres_container_for_port)"; then
    echo "    Restarting stopped container ${stopped} on port ${db_port}"
    docker start "$stopped" >/dev/null
    postgres_container="$stopped"
  else
    docker compose up -d postgres
    postgres_container="$(docker compose ps -q postgres 2>/dev/null | head -n 1)"
    if [ -n "$postgres_container" ]; then
      postgres_container="$(docker inspect --format '{{.Name}}' "$postgres_container" 2>/dev/null | sed 's#^/##')"
    fi
  fi

  if [ -z "$postgres_container" ]; then
    echo "No PostgreSQL container found on localhost:${db_port} after compose up."
    exit 1
  fi

  echo "==> Waiting for PostgreSQL to be ready..."
  until exec_postgres "$postgres_container" pg_isready -U "$POSTGRES_USER" -d postgres > /dev/null 2>&1; do
    sleep 1
  done

  echo "==> Ensuring database '$POSTGRES_DB' exists..."
  db_exists="$(exec_postgres "$postgres_container" \
    psql -U "$POSTGRES_USER" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DB'")"

  if [ "$db_exists" != "1" ]; then
    exec_postgres "$postgres_container" \
      psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 \
      -c "CREATE DATABASE \"$POSTGRES_DB\"" \
      > /dev/null
  fi

  echo "✓ PostgreSQL ready (local Docker). Database: $POSTGRES_DB"
else
  # ---------- Remote: skip Docker, verify connectivity ----------
  echo "==> Remote database detected (host: $db_host). Skipping Docker."
  if command -v pg_isready > /dev/null 2>&1; then
    echo "==> Waiting for PostgreSQL at $db_host:$db_port to be ready..."
    until pg_isready -d "$DATABASE_URL" > /dev/null 2>&1; do
      sleep 1
    done
    echo "✓ PostgreSQL ready (remote: $db_host:$db_port). Database: $db_name"
  else
    echo "==> pg_isready not found. Skipping remote connectivity preflight."
    echo "✓ PostgreSQL configured (remote: $db_host:$db_port). Database: $db_name"
  fi
fi
