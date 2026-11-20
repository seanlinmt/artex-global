#!/usr/bin/env bash
# ARTEX macOS launcher runtime for this checkout.
#
# The app bundle in /Applications only checks the single-instance lock and asks
# Terminal to run this script, so logs stay visible and Ctrl-C stops the stack
# through dev.sh's process-group teardown.
#
# This checkout has no start-dev.sh: dev.sh runs the backend (:8787), the
# traffic proxy (:8788) and next dev (:5173), but it does not provision
# PostgreSQL and it does not export a DSN, so the backend exits at startup
# without one. This script supplies both before handing over to dev.sh.
#
# Environment overrides:
#   ARTEX_ROOT                     repo root (default: two levels above this script)
#   ARTEX_LAUNCHER_LOCK_DIR        lock root (default: ~/Library/Application Support/ARTEX)
#   ARTEX_LAUNCHER_UI_PORT         UI port (default: 5173)
#   ARTEX_LAUNCHER_BACKEND_PORT    backend port (default: 8787)
#   ARTEX_LAUNCHER_PROXY_PORT      traffic proxy port (default: 8788)
#   ARTEX_DEV_PG_PORT              PostgreSQL port (default: 5433)
#   ARTEX_LAUNCHER_NONINTERACTIVE  1 = never call osascript, always take the default button
#   ARTEX_LAUNCHER_NO_BROWSER      1 = do not open the UI in a browser
#
# bash 3.2 (system bash) only: no flock, no associative arrays, no mapfile.

ARTEX_ROOT="${ARTEX_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
UI_PORT="${ARTEX_LAUNCHER_UI_PORT:-5173}"
BACKEND_PORT="${ARTEX_LAUNCHER_BACKEND_PORT:-8787}"
PROXY_PORT="${ARTEX_LAUNCHER_PROXY_PORT:-8788}"
PG_PORT="${ARTEX_DEV_PG_PORT:-5433}"
PG_CONTAINER="${ARTEX_DEV_PG_CONTAINER:-artex-dev-postgres}"
PG_VOLUME="${ARTEX_DEV_PG_VOLUME:-artex-dev-pgdata}"
LOCK_DIR="${ARTEX_LAUNCHER_LOCK_DIR:-$HOME/Library/Application Support/ARTEX}"
LOCK_PATH="$LOCK_DIR/launcher.lock"
NONINTERACTIVE="${ARTEX_LAUNCHER_NONINTERACTIVE:-0}"

msg() { printf '[*] %s\n' "$*"; }
warn() { printf '[!] %s\n' "$*" >&2; }
err() { printf '[x] %s\n' "$*" >&2; }

# Escape a string for inclusion inside an AppleScript double-quoted literal.
applescript_quote() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# Print the PID recorded in the lock when that process is alive; print nothing
# when there is no lock, the file is malformed, or the holder is dead.
lock_holder_pid() {
  local pidfile="$LOCK_PATH/pid" pid
  [ -f "$pidfile" ] || return 0
  pid=$(cat "$pidfile" 2>/dev/null) || return 0
  case "$pid" in
  '' | *[!0-9]*) return 0 ;;
  esac
  if kill -0 "$pid" 2>/dev/null; then
    printf '%s\n' "$pid"
  fi
  return 0
}

# 0 = acquired (pid written), 1 = held by another live process, 2 = cannot create lock root.
lock_acquire() {
  local pid
  mkdir -p "$LOCK_DIR" 2>/dev/null || return 2
  if [ -d "$LOCK_PATH" ]; then
    pid=$(lock_holder_pid)
    if [ -n "$pid" ] && [ "$pid" != "$$" ]; then
      return 1
    fi
    rm -rf "$LOCK_PATH" 2>/dev/null || true
  fi
  if mkdir "$LOCK_PATH" 2>/dev/null; then
    printf '%s\n' "$$" >"$LOCK_PATH/pid"
    return 0
  fi
  # Lost a mkdir race: report whoever won it.
  pid=$(lock_holder_pid)
  if [ -n "$pid" ] && [ "$pid" != "$$" ]; then
    return 1
  fi
  return 0
}

# Remove the lock only when this process owns it.
lock_release() {
  local pidfile="$LOCK_PATH/pid"
  if [ -f "$pidfile" ]; then
    [ "$(cat "$pidfile" 2>/dev/null)" = "$$" ] || return 0
  fi
  rm -rf "$LOCK_PATH" 2>/dev/null || true
}

# 0 = something is listening on the port.
port_busy() {
  local port="$1"
  lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

# 0 = web dependencies must be (re)installed before the UI can start.
needs_npm_ci() {
  local root="$1"
  local web="$root/web"
  [ -d "$web/node_modules" ] || return 0
  [ -f "$web/package-lock.json" ] || return 1
  [ -f "$web/node_modules/.package-lock.json" ] || return 0
  [ "$web/package-lock.json" -nt "$web/node_modules/.package-lock.json" ]
}

# Print the chosen button. Non-interactive (or no osascript) prints the default.
# Usage: ui_dialog <message> <default-button> <button>...
ui_dialog() {
  local message="$1" default="$2" out b list="" first=1
  shift 2
  if [ "$NONINTERACTIVE" = "1" ] || ! command -v osascript >/dev/null 2>&1; then
    printf '%s\n' "$default"
    return 0
  fi
  for b in "$@"; do
    if [ "$first" = "1" ]; then
      first=0
    else
      list="$list, "
    fi
    list="$list\"$(applescript_quote "$b")\""
  done
  out=$(osascript \
    -e "display dialog \"$(applescript_quote "$message")\" buttons {$list} default button \"$(applescript_quote "$default")\" with title \"ARTEX\" with icon note" \
    2>/dev/null) || return 1
  case "$out" in
  button\ returned:*) printf '%s\n' "${out#button returned:}" ;;
  *) printf '%s\n' "$default" ;;
  esac
}

open_ui() {
  open "http://localhost:$UI_PORT" >/dev/null 2>&1 || true
}

pause_if_interactive() {
  [ "$NONINTERACTIVE" = "1" ] && return 0
  if [ -t 0 ]; then
    printf 'Press any key to close…'
    read -r -n 1 _ || true
    printf '\n'
  fi
}

cleanup() {
  trap '' INT TERM
  if [ -n "${watchdog_pid:-}" ]; then
    kill "$watchdog_pid" 2>/dev/null || true
  fi
  if [ -n "${ui_poll_pid:-}" ]; then
    kill "$ui_poll_pid" 2>/dev/null || true
  fi
  lock_release
}

# Bring up the PostgreSQL dev container dev.sh expects a DSN for. Idempotent:
# reuses a running container, starts a stopped one, creates a missing one.
ensure_pg() {
  local state
  # docker inspect prints an empty line and exits non-zero when the container is
  # absent, so trim whitespace before matching; otherwise a missing container
  # lands in the "start the existing one" branch and fails.
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
  local i
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
  local pid port choice status tool
  ARTEX_ROOT="${ARTEX_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

  if ! cd "$ARTEX_ROOT" 2>/dev/null; then
    err "ARTEX checkout not found at $ARTEX_ROOT"
    pause_if_interactive
    return 1
  fi
  if [ ! -f ./dev.sh ]; then
    err "dev.sh is missing in $ARTEX_ROOT"
    pause_if_interactive
    return 1
  fi

  trap cleanup EXIT

  lock_acquire
  case $? in
  0) ;;
  1)
    pid=$(lock_holder_pid)
    msg "ARTEX is already running (PID ${pid:-unknown}); not starting a second stack"
    choice=$(ui_dialog "ARTEX is already running (PID ${pid:-unknown})." "Open UI" "Open UI" "OK") || choice="OK"
    [ "$choice" = "Open UI" ] && open_ui
    return 0
    ;;
  *)
    err "cannot create the lock directory $LOCK_DIR"
    pause_if_interactive
    return 1
    ;;
  esac

  # dev.sh kills its whole process group on shutdown, which also TERMs this
  # script. Rather than trap those signals here, a watchdog ignores them and
  # removes the lock once this process is gone, so a stopped stack never blocks
  # the next launch.
  (
    trap '' INT TERM HUP
    while kill -0 "$$" 2>/dev/null; do
      sleep 1
    done
    rm -rf "$LOCK_PATH"
  ) &
  watchdog_pid=$!

  for port in "$BACKEND_PORT" "$PROXY_PORT" "$UI_PORT"; do
    if port_busy "$port"; then
      msg "port $port is already in use — a stack is running"
      choice=$(ui_dialog "Port $port is already in use, so a stack appears to be running." "Open UI" "Open UI" "OK") || choice="OK"
      [ "$choice" = "Open UI" ] && open_ui
      return 0
    fi
  done

  for tool in docker go npm; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      err "$tool is required but not on PATH"
      pause_if_interactive
      return 1
    fi
  done

  if needs_npm_ci "$ARTEX_ROOT"; then
    msg "installing web dependencies (package-lock.json changed or node_modules missing)"
    if ! (cd "$ARTEX_ROOT/web" && npm ci --no-audit --no-fund); then
      err "npm ci failed; the UI would not start correctly"
      pause_if_interactive
      return 1
    fi
  fi

  if ! ensure_pg; then
    pause_if_interactive
    return 1
  fi

  # dev.sh does not set either of these; without the DSN the backend exits at
  # startup, and without PORT next dev binds :3000 instead of the documented UI.
  export ARTEX_PG_DSN="postgres://artex:artex@127.0.0.1:${PG_PORT}/artex?sslmode=disable"
  export PORT="$UI_PORT"

  if [ "${ARTEX_LAUNCHER_NO_BROWSER:-0}" != "1" ] && command -v curl >/dev/null 2>&1; then
    (
      i=0
      while [ "$i" -lt 180 ]; do
        if curl -sf -o /dev/null "http://localhost:$UI_PORT"; then
          open_ui
          exit 0
        fi
        sleep 1
        i=$((i + 1))
      done
    ) &
    ui_poll_pid=$!
  fi

  msg "starting the dev stack (backend :$BACKEND_PORT / proxy :$PROXY_PORT / UI :$UI_PORT)"
  ./dev.sh
  status=$?
  msg "stack stopped (status $status)"
  return "$status"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
