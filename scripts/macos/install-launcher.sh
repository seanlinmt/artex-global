#!/usr/bin/env bash
# Build, sign, notarise and install ARTEX.app for macOS.
#
# The installed app is a thin shell shim that execs
# scripts/macos/artex-app-run.sh, which in turn execs the single embedded-frontend
# binary at <repo>/artex. Nothing inside the bundle needs rebuilding after a
# `git pull`: update the checkout, run scripts/macos/rebuild-artex.sh, and
# relaunch. See docs/macos-install-en.md.
#
# Usage:
#   scripts/macos/install-launcher.sh [--app-dir DIR] [--desktop]
#                                     [--repo-root DIR] [--no-notarize]
#                                     [--uninstall]
#
#   --app-dir DIR     install destination (default: /Applications, falling back
#                     to ~/Applications when /Applications is not writable)
#   --desktop         also put a Desktop link to the app
#   --repo-root DIR   checkout the app should launch (default: this repository)
#   --no-notarize     sign only; skip notarytool submission and stapling
#   --uninstall       remove the app, the Desktop link and any stale lock
#
# Signing uses the first "Developer ID Application" identity in the keychain, or
# ARTEX_CODESIGN_IDENTITY when set. Notarisation reads NOTARY_KEY_PATH,
# NOTARY_KEY_ID and NOTARY_ISSUER_ID (App Store Connect API key) from the
# environment. Without credentials the app stays signed but not notarised, and
# the installer says so.
set -euo pipefail

APP_NAME="ARTEX"
BUNDLE_ID="org.clearpixels.artex.launcher"
APP_DIR="/Applications"
MAKE_DESKTOP=0
UNINSTALL=0
NOTARIZE=1
NOTARIZED=0
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

info() { printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok() { printf '\033[32m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }
die() {
  printf '\033[31m[x]\033[0m %s\n' "$*" >&2
  exit 1
}

usage() {
  sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
  --app-dir) APP_DIR="${2:?--app-dir needs a value}"; shift 2 ;;
  --app-dir=*) APP_DIR="${1#*=}"; shift ;;
  --desktop) MAKE_DESKTOP=1; shift ;;
  --repo-root) REPO_ROOT="${2:?--repo-root needs a value}"; shift 2 ;;
  --repo-root=*) REPO_ROOT="${1#*=}"; shift ;;
  --no-notarize) NOTARIZE=0; shift ;;
  --uninstall) UNINSTALL=1; shift ;;
  -h | --help) usage; exit 0 ;;
  *) die "unknown option: $1 (try --help)" ;;
  esac
done

case "$APP_DIR" in
"~/"*) APP_DIR="$HOME/${APP_DIR#\~/}" ;;
esac
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

# POSIX single-quoted string for embedding a path in shell code.
shell_squote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# Escape for an AppleScript double-quoted literal.
applescript_quote() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# Escape a string for use in a sed s|...|...|g replacement.
sed_escape() {
  printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

uninstall() {
  local app="$APP_DIR/$APP_NAME.app" link="$HOME/Desktop/$APP_NAME.app" lock pid
  if [ -e "$app" ]; then
    rm -rf "$app"
    ok "removed $app"
  else
    warn "nothing installed at $app"
  fi
  if [ -L "$link" ]; then
    rm -f "$link"
    ok "removed $link"
  fi
  lock="$HOME/Library/Application Support/ARTEX/launcher.lock"
  if [ -d "$lock" ]; then
    pid=$(cat "$lock/pid" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      warn "lock is held by a running stack (PID $pid); left in place"
    else
      rm -rf "$lock"
      ok "removed stale launcher lock"
    fi
  fi
}

find_identity() {
  if [ -n "${ARTEX_CODESIGN_IDENTITY:-}" ]; then
    printf '%s\n' "$ARTEX_CODESIGN_IDENTITY"
    return 0
  fi
  security find-identity -v -p codesigning 2>/dev/null |
    awk -F'"' '/Developer ID Application/ { print $2; exit }'
}

make_icon() {
  local src="$REPO_ROOT/web/public/logo.png" iconset="$tmpdir/AppIcon.iconset" size
  if [ ! -f "$src" ]; then
    warn "web/public/logo.png not found; installing without an icon"
    return 0
  fi
  if ! command -v sips >/dev/null 2>&1 || ! command -v iconutil >/dev/null 2>&1; then
    warn "sips/iconutil missing; installing without an icon"
    return 0
  fi
  mkdir -p "$iconset"
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" "$src" --out "$iconset/icon_${size}x${size}.png" >/dev/null 2>&1 || {
      warn "sips failed at ${size}px; installing without an icon"
      return 0
    }
    sips -z $((size * 2)) $((size * 2)) "$src" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null 2>&1 || {
      warn "sips failed at ${size}px@2x; installing without an icon"
      return 0
    }
  done
  if ! iconutil -c icns "$iconset" -o "$tmpdir/AppIcon.icns" 2>/dev/null; then
    warn "iconutil failed; installing without an icon"
    return 0
  fi
  cp "$tmpdir/AppIcon.icns" "$tmpdir/$APP_NAME.app/Contents/Resources/AppIcon.icns"
}

write_info_plist() {
  cat >"$tmpdir/$APP_NAME.app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>ARTEX</string>
  <key>CFBundleDisplayName</key>
  <string>ARTEX</string>
  <key>CFBundleIdentifier</key>
  <string>org.clearpixels.artex.launcher</string>
  <key>CFBundleExecutable</key>
  <string>launcher</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0</string>
  <key>CFBundleVersion</key>
  <string>1.0</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST
}

write_launcher() {
  local root_q
  root_q=$(shell_squote "$REPO_ROOT")
  # The bundle's executable is a tiny shell shim that execs the runtime script.
  # `exec` (not Terminal + do script) is deliberate: it keeps the process
  # identity of ARTEX.app, so macOS TCC attributes Full Disk Access to the app
  # rather than to a bare shell, which macOS auto-denies without prompting.
  cat >"$tmpdir/$APP_NAME.app/Contents/MacOS/launcher" <<LAUNCHER
#!/bin/bash
# Generated by scripts/macos/install-launcher.sh — do not edit.
# Re-run the installer after moving the checkout.
APP_ROOT=$root_q
RUN="\$APP_ROOT/scripts/macos/artex-app-run.sh"
if [ ! -d "\$APP_ROOT" ]; then
  osascript -e 'display dialog "ARTEX checkout not found at: $REPO_ROOT. Re-run scripts/macos/install-launcher.sh." buttons {"OK"} default button "OK" with title "ARTEX" with icon stop' >/dev/null 2>&1 || true
  exit 1
fi
if [ ! -x "\$RUN" ]; then
  osascript -e 'display dialog "Launcher runtime missing: $REPO_ROOT/scripts/macos/artex-app-run.sh" buttons {"OK"} default button "OK" with title "ARTEX" with icon stop' >/dev/null 2>&1 || true
  exit 1
fi
export ARTEX_ROOT="\$APP_ROOT"
exec "\$RUN" "\$@"
LAUNCHER
  chmod +x "$tmpdir/$APP_NAME.app/Contents/MacOS/launcher"
}

sign_bundle() {
  local app="$tmpdir/$APP_NAME.app" identity
  identity=$(find_identity)
  if [ -z "$identity" ]; then
    warn "no Developer ID Application identity found; signing ad-hoc"
    codesign --force --sign - "$app"
    return 0
  fi
  info "signing with: $identity"
  if ! codesign --force --options runtime --timestamp --sign "$identity" "$app"; then
    warn "timestamped signing failed (offline?); retrying without --timestamp"
    codesign --force --options runtime --sign "$identity" "$app"
  fi
}

notarize_bundle() {
  local app="$tmpdir/$APP_NAME.app" zip="$tmpdir/$APP_NAME.zip" out id
  if [ "$NOTARIZE" != "1" ]; then
    warn "--no-notarize: app is signed but not notarised"
    return 0
  fi
  if [ -z "${NOTARY_KEY_PATH:-}" ] || [ -z "${NOTARY_KEY_ID:-}" ] || [ -z "${NOTARY_ISSUER_ID:-}" ]; then
    warn "NOTARY_KEY_PATH / NOTARY_KEY_ID / NOTARY_ISSUER_ID not set; app is signed but not notarised"
    return 0
  fi
  if [ ! -f "$NOTARY_KEY_PATH" ]; then
    warn "notary key not found at $NOTARY_KEY_PATH; app is signed but not notarised"
    return 0
  fi
  ditto -c -k --keepParent "$app" "$zip"
  info "submitting to the Apple notary service (can take a few minutes)"
  if ! out=$(xcrun notarytool submit "$zip" \
    --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER_ID" \
    --wait 2>&1); then
    printf '%s\n' "$out" | sed 's/^/    /'
    die "notarytool submit failed"
  fi
  printf '%s\n' "$out" | sed 's/^/    /'
  case "$out" in
  *"status: Accepted"*) ;;
  *)
    id=$(printf '%s\n' "$out" | awk '/^ *id:/ { print $2; exit }')
    if [ -n "$id" ]; then
      warn "notary log for submission $id:"
      xcrun notarytool log "$id" --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER_ID" 2>&1 | sed 's/^/    /'
    fi
    die "notarisation was not accepted"
    ;;
  esac
  xcrun stapler staple "$app"
  xcrun stapler validate "$app"
  NOTARIZED=1
  ok "notarised and stapled"
}

install_bundle() {
  local app="$APP_DIR/$APP_NAME.app" current
  mkdir -p "$APP_DIR" 2>/dev/null || die "cannot create $APP_DIR"
  if [ -e "$app" ]; then
    current=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Contents/Info.plist" 2>/dev/null || true)
    if [ "$current" != "$BUNDLE_ID" ]; then
      die "$app exists and is not ARTEX (identifier: ${current:-none})"
    fi
    rm -rf "$app"
  fi
  ditto "$tmpdir/$APP_NAME.app" "$app"
  ok "installed $app"
  if [ "$MAKE_DESKTOP" = "1" ]; then
    ln -sfn "$app" "$HOME/Desktop/$APP_NAME.app"
    ok "linked $HOME/Desktop/$APP_NAME.app"
  fi
}

verify_installed() {
  local app="$APP_DIR/$APP_NAME.app" out
  codesign --verify --strict "$app" || die "codesign verification failed for $app"
  ok "codesign --verify --strict passed"
  codesign -dvv "$app" 2>&1 | sed 's/^/    /'
  if [ "$NOTARIZED" = "1" ]; then
    if out=$(spctl --assess --type execute --verbose=4 "$app" 2>&1); then
      printf '%s\n' "$out" | sed 's/^/    /'
      ok "Gatekeeper assessment passed"
    else
      printf '%s\n' "$out" | sed 's/^/    /'
      warn "Gatekeeper assessment did not pass; the app still runs because it is not quarantined"
    fi
  fi
}

if [ "$UNINSTALL" = "1" ]; then
  uninstall
  exit 0
fi

if [ ! -x "$REPO_ROOT/scripts/macos/artex-launch.sh" ]; then
  die "scripts/macos/artex-launch.sh not found or not executable under $REPO_ROOT"
fi
# This checkout launches through dev.sh; artex-launch.sh provisions the
# PostgreSQL container and the DSN that dev.sh expects.
if [ ! -f "$REPO_ROOT/dev.sh" ]; then
  die "dev.sh not found under $REPO_ROOT"
fi

if [ ! -d "$APP_DIR" ]; then
  mkdir -p "$APP_DIR" 2>/dev/null || APP_DIR=""
fi
if [ -z "$APP_DIR" ] || [ ! -w "$APP_DIR" ]; then
  warn "${APP_DIR:-the requested app dir} is not writable; installing to $HOME/Applications instead"
  APP_DIR="$HOME/Applications"
  mkdir -p "$APP_DIR" || die "cannot create $APP_DIR"
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/$APP_NAME.app/Contents/MacOS" "$tmpdir/$APP_NAME.app/Contents/Resources"
write_info_plist
write_launcher
make_icon
sign_bundle
notarize_bundle
install_bundle
verify_installed

info "launch with: open $APP_DIR/$APP_NAME.app"
info "the app runs this checkout: $REPO_ROOT"
info "update with a git pull in the checkout, then relaunch; re-run this installer only if the checkout moves"
info "uninstall with: $0 --uninstall"
