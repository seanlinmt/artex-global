#!/bin/bash
# ARTEX macOS app runtime: starts the single embedded-frontend binary.
#
# This is what /Applications/ARTEX.app execs. It deliberately uses `exec` all
# the way down so the launched process keeps the app bundle's identity: macOS
# TCC attributes Full Disk Access to the *responsible process*, and any extra
# bash/Terminal hop in the chain would make TCC attribute the read to that
# shell instead of to ARTEX.app (a bare shell is auto-denied without a prompt).
#
# What it does:
#   1. If the backend port is already listening, just open the UI and exit.
#   2. Make sure the PostgreSQL dev container is up.
#   3. exec the built binary with the embedded frontend.
#
# Environment overrides:
#   ARTEX_ROOT              repo root (default: two levels above this script)
#   ARTEX_APP_BACKEND_PORT  backend/UI port (default: 8787)
#   ARTEX_APP_PROXY_PORT    traffic proxy port (default: 8788)
#   ARTEX_DEV_PG_PORT       PostgreSQL port (default: 5433)
#   ARTEX_APP_PG_CONTAINER  postgres container name (default: artex-dev-postgres)
#   ARTEX_APP_PG_VOLUME     postgres volume name (default: artex-dev-pgdata)
#   ARTEX_APP_NO_BROWSER    1 = do not open the UI
#   ARTEX_APP_LOG           log file (default: ~/Library/Logs/ARTEX/artex.log)
#
# bash 3.2 (system bash) only: no flock, no associative arrays, no mapfile.

ARTEX_ROOT="${ARTEX_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
BACKEND_PORT="${ARTEX_APP_BACKEND_PORT:-8787}"
PROXY_PORT="${ARTEX_APP_PROXY_PORT:-8788}"
PG_PORT="${ARTEX_DEV_PG_PORT:-5433}"
PG_CONTAINER="${ARTEX_APP_PG_CONTAINER:-artex-dev-postgres}"
PG_VOLUME="${ARTEX_APP_PG_VOLUME:-artex-dev-pgdata}"
LOG_DIR="${ARTEX_APP_LOG_DIR:-$HOME/Library/Logs/ARTEX}"
LOG_FILE="${ARTEX_APP_LOG:-$LOG_DIR/artex.log}"
UI_URL="http://localhost:$BACKEND_PORT"
BINARY="$ARTEX_ROOT/artex"

msg() { printf '[*] %s\n' "$*"; }
warn() { printf '[!] %s\n' "$*" >&2; }
err() { printf '[x] %s\n' "$*" >&2; }

# Show a dialog when launched from Finder, where stderr is invisible.
notify() {
  osascript -e "display dialog \"$1\" buttons {\"OK\"} default button \"OK\" with title \"ARTEX\" with icon ${2:-note}" \
    >/dev/null 2>&1 || true
}

port_busy() {
  lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
}

open_ui() {
  [ "${ARTEX_APP_NO_BROWSER:-0}" = "1" ] && return 0
  open "$UI_URL" >/dev/null 2>&1 || true
}

# Bring up the PostgreSQL dev container. Idempotent: reuse running, start
# stopped, create missing. Prints a clear error instead of dying silently when
# Docker itself is unavailable.
ensure_pg() {
  local state i
  if ! command -v docker >/dev/null 2>&1; then
    err "docker is not on PATH; PostgreSQL cannot be started"
    return 1
  fi
  state=$(docker inspect -f '{{.State.Running}}' "$PG_CONTAINER" 2>/dev/null | tr -d '[:space:]')
  case "$state" in
  true) return 0 ;;
  false)
    msg "starting existing PostgreSQL dev container $PG_CONTAINER"
    docker start "$PG_CONTAINER" >/dev/null || return 1
    ;;
  *)
    msg "creating PostgreSQL dev container $PG_CONTAINER on 127.0.0.1:$PG_PORT"
    docker run -d --name "$PG_CONTAINER" \
      -e POSTGRES_USER=artex \
      -e POSTGRES_PASSWORD=artex \
      -e POSTGRES_DB=artex \
      -v "$PG_VOLUME:/var/lib/postgresql/data" \
      -p "127.0.0.1:${PG_PORT}:5432" postgres:16-alpine >/dev/null || return 1
    ;;
  esac
  msg "waiting for PostgreSQL to accept connections"
  for i in $(seq 1 60); do
    if docker exec "$PG_CONTAINER" pg_isready -U artex -d artex >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  err "PostgreSQL container $PG_CONTAINER did not become ready"
  return 1
}

main() {
  set -uo pipefail

  # Finder-launched apps inherit a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin),
  # which does not include Homebrew or Docker Desktop's CLI. Add the usual
  # locations so docker/lsof resolve the same way they do in a login shell.
  case ":$PATH:" in
  *":/opt/homebrew/bin:"*) ;;
  *) PATH="/opt/homebrew/bin:/usr/local/bin:$PATH" ;;
  esac
  export PATH

  # A stack is already up (this app, or a dev stack on the same port): just
  # surface the UI instead of starting a second one.
  if port_busy "$BACKEND_PORT"; then
    msg "ARTEX is already listening on :$BACKEND_PORT — opening the UI"
    open_ui
    return 0
  fi

  if [ ! -d "$ARTEX_ROOT" ]; then
    err "ARTEX checkout not found at $ARTEX_ROOT"
    notify "ARTEX checkout not found at: $ARTEX_ROOT. Re-run scripts/macos/rebuild-artex.sh."
    return 1
  fi

  if [ ! -x "$BINARY" ]; then
    err "built binary missing at $BINARY"
    notify "The ARTEX binary is missing. Run scripts/macos/rebuild-artex.sh to build it."
    return 1
  fi

  if ! ensure_pg; then
    notify "PostgreSQL could not be started. Is Docker running?"
    return 1
  fi

  mkdir -p "$LOG_DIR" 2>/dev/null || true

  # Export the DSN the binary reads. A config.json beside the binary takes
  # precedence only for skill_dir/language; the DSN is simplest as an env var.
  export ARTEX_PG_DSN="postgres://artex:artex@127.0.0.1:${PG_PORT}/artex?sslmode=disable"

  msg "starting ARTEX ($BINARY) on :$BACKEND_PORT — logging to $LOG_FILE"
  # exec keeps this process image's identity, so TCC attributes Full Disk Access
  # to ARTEX.app rather than to this script's shell.
  exec "$BINARY" -addr "127.0.0.1:$BACKEND_PORT" -proxy "127.0.0.1:$PROXY_PORT" \
    >>"$LOG_FILE" 2>&1
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
