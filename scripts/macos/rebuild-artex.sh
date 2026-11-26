#!/usr/bin/env bash
# Rebuild the ARTEX single binary after a `git pull`, and re-sign it with the
# same Developer ID identity so the Full Disk Access grant survives.
#
# Why re-signing matters: macOS TCC records a Full Disk Access grant against the
# binary's code requirement. Ad-hoc or unsigned binaries get a new identity on
# every rebuild and lose the grant. Signing with a stable Developer ID and a
# stable identifier keeps the Designated Requirement unchanged across rebuilds,
# so the grant you approved once stays valid after each update.
#
# Usage:
#   scripts/macos/rebuild-artex.sh [--pull] [--with-frontend] [--skip-frontend]
#                                  [--identity NAME] [--no-sign] [--restart]
#
#   --pull            run `git pull --ff-only` first
#   --with-frontend   force a frontend rebuild even if it looks up to date
#   --skip-frontend   never rebuild the frontend (backend-only changes)
#   --identity NAME   codesign identity (default: first "Developer ID Application")
#   --no-sign         leave the binary ad-hoc signed (TCC grant will NOT persist)
#   --restart         restart the app after a successful build
#
# Default frontend policy: rebuild only when the web sources are newer than the
# exported frontend in server/webui/dist. A backend-only pull therefore builds
# in seconds instead of minutes.
#
# Environment:
#   ARTEX_CODESIGN_IDENTITY   same as --identity
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="$PWD"

PULL=0
FRONTEND=auto
IDENTITY="${ARTEX_CODESIGN_IDENTITY:-}"
SIGN=1
RESTART=0

info() { printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok() { printf '\033[32m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
die() {
  printf '\033[31m[x]\033[0m %s\n' "$*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
  --pull) PULL=1 ;;
  --with-frontend) FRONTEND=force ;;
  --skip-frontend) FRONTEND=skip ;;
  --identity)
    shift
    [ $# -gt 0 ] || die "--identity needs a value"
    IDENTITY="$1"
    ;;
  --no-sign) SIGN=0 ;;
  --restart) RESTART=1 ;;
  -h | --help)
    sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  *) die "unknown option: $1" ;;
  esac
  shift
done

# --- 1. optionally update the checkout -------------------------------------
if [ "$PULL" = "1" ]; then
  info "git pull --ff-only"
  git pull --ff-only
  ok "checkout updated"
fi

# --- 2. decide whether the frontend must be rebuilt ------------------------
# The exported frontend lives in server/webui/dist and is embedded into the
# binary. Rebuild it when any tracked web source is newer than the export.
frontend_stale() {
  local stamp="server/webui/dist/index.html" newer
  [ -f "$stamp" ] || return 0
  # Compare against tracked web sources only. Skip installed deps, the generated
  # export, and the .next build cache — all of which change without implying the
  # frontend needs rebuilding.
  newer=$(find web \
    \( -path web/node_modules -o -path web/out -o -path web/.next \) -prune -o \
    -type f -newer "$stamp" -print 2>/dev/null | head -1)
  [ -n "$newer" ]
}

build_frontend() {
  info "building frontend (npm ci --include=dev; build:static)"
  # --include=dev: if NODE_ENV=production is set in the caller's shell, npm ci
  # would skip devDependencies and @tailwindcss/postcss would be missing.
  ( cd web && npm ci --include=dev --no-audit --no-fund && npm run build:static ) \
    || die "frontend build failed"
  info "syncing web/out into server/webui/dist"
  # Replace the tree rather than copy into it, so stale hashed assets from a
  # previous build cannot linger and be served.
  rm -rf server/webui/dist
  mkdir -p server/webui/dist
  cp -a web/out/. server/webui/dist/
  ok "frontend rebuilt"
}

case "$FRONTEND" in
force) build_frontend ;;
skip) info "skipping frontend rebuild (--skip-frontend)" ;;
auto)
  if frontend_stale; then
    build_frontend
  else
    info "frontend export is up to date; skipping rebuild"
  fi
  ;;
esac

[ -f server/webui/dist/index.html ] || die "server/webui/dist is empty; run with --with-frontend"

# --- 3. build the binary ---------------------------------------------------
info "building artex (embedui, CGO_ENABLED=0)"
CGO_ENABLED=0 go build -tags embedui -o artex.new ./cmd/artex || die "go build failed"

# --- 4. sign it ------------------------------------------------------------
if [ "$SIGN" = "1" ]; then
  if [ -z "$IDENTITY" ]; then
    IDENTITY=$(security find-identity -v -p codesigning 2>/dev/null \
      | awk -F'"' '/Developer ID Application/ { print $2; exit }')
  fi
  if [ -z "$IDENTITY" ]; then
    warn "no Developer ID Application identity found; signing ad-hoc"
    warn "the Full Disk Access grant will NOT survive the next rebuild"
    codesign --force --sign - --identifier "org.clearpixels.artex" artex.new
  else
    info "signing with: $IDENTITY"
    # Stable --identifier is what keeps the Designated Requirement (and the TCC
    # grant) identical across rebuilds. Do not change it.
    codesign --force --options runtime --timestamp \
      --identifier "org.clearpixels.artex" --sign "$IDENTITY" artex.new \
      || die "codesign failed"
    codesign --verify --strict --verbose=2 artex.new || die "signature verification failed"
    ok "signed and verified"
  fi
else
  warn "--no-sign: leaving artex.new ad-hoc signed"
fi

# --- 5. swap in the new binary --------------------------------------------
# Rename rather than overwrite: a running instance keeps its own inode, so the
# swap is safe even while ARTEX is up.
mv -f artex.new artex
chmod +x artex
ok "built $ROOT/artex"

# --- 6. optionally restart ------------------------------------------------
if [ "$RESTART" = "1" ]; then
  info "restarting ARTEX"
  osascript -e 'tell application "ARTEX" to quit' >/dev/null 2>&1 || true
  sleep 2
  pkill -f "$ROOT/artex" >/dev/null 2>&1 || true
  sleep 1
  open -a ARTEX 2>/dev/null || "$ROOT/scripts/macos/artex-app-run.sh" &
  ok "restart requested"
fi

echo
ok "done. If you changed the signing identity, re-approve Full Disk Access in"
echo "   System Settings → Privacy & Security → Full Disk Access."
