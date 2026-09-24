# Multica fork — production self-host lifecycle (wraps scripts/dev-env.sh).
# Install: winget install Casey.Just
#          Git for Windows (bash) — https://git-scm.com/download/win
#
# On Windows PowerShell, recipes call scripts/just-dev-env.ps1 so paths match the
# Git Bash registry used by `make up` (/c/Users/...), not WSL (/mnt/c/...).
#
# List recipes:  just --list
# First time:    just build          (compile server + Next.js)
# Start stack:   just up             (production binaries, not dev servers)
# Full recycle:  just refresh        (stop, then start again)
# Status:        just status
#
# Hot reload for active development: make up  (or make dev)

set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set windows-shell := ["cmd.exe", "/c"]

root := justfile_directory()
components := "api,web,daemon"

default:
    @just --list

# Build production artifacts: Go server/CLI + Next.js web + cursor_sdk executor.
[unix]
build:
    bash "{{root}}/scripts/just-build.sh"

[windows]
build:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action build

# Start compiled API, production Next.js, and agent daemon for this checkout.
[unix]
up:
    bash "{{root}}/scripts/dev-env.sh" up --production --components {{components}}

[windows]
up:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action up

start:
    @just up

# Stop API, web, and daemon. Database, ports, and CLI profile are kept.
[unix]
down:
    bash "{{root}}/scripts/dev-env.sh" down --components {{components}}

[windows]
down:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action down

stop:
    @just down

# Stop everything, then start again — one command to recycle the fork stack.
[unix]
restart:
    bash "{{root}}/scripts/dev-env.sh" down --components {{components}}
    bash "{{root}}/scripts/dev-env.sh" up --production --components {{components}}

[windows]
restart:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action restart

refresh:
    @just restart

# Stop, build production artifacts, then start again.
rebuild-restart:
    @just down
    @just build
    @just up

# Show ports, database, and which components are running for this checkout.
[unix]
status:
    bash "{{root}}/scripts/dev-env.sh" status

[windows]
status:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action status

# List every dev environment registered on this machine.
[unix]
list:
    bash "{{root}}/scripts/dev-env.sh" list

[windows]
list:
    powershell -NoProfile -ExecutionPolicy Bypass -File "{{root}}\scripts\just-dev-env.ps1" -Action list
