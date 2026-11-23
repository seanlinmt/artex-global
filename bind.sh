#!/usr/bin/env bash
# ARTEX binding selection script.
#
# The default docker-compose.yml publishes only on 127.0.0.1 (to prevent remote exposure). This script changes
# that publish address to "the one interface you choose now". That is, it exposes only LAN or Wi-Fi,
# and the other interface does not accept incoming connections.
#
# Usage:
#   ./bind.sh list              Show the available interfaces and IPv4 addresses
#   ./bind.sh local             Revert to 127.0.0.1 (default, local-only)
#   ./bind.sh lan               Publish on the LAN interface address
#   ./bind.sh wifi              Publish on the Wi-Fi interface address
#   ./bind.sh any               Publish on all interfaces (0.0.0.0)
#   ./bind.sh en12              Specify an interface name directly
#   ./bind.sh 192.168.20.22     Specify an IPv4 address directly
#   ./bind.sh status            Show the current binding and publish status
#
# lan/wifi find the target in this order:
#   1) environment variables ARTEX_LAN_IFACE / ARTEX_WIFI_IFACE (or the same names in .env)
#   2) automatic detection (macOS: networksetup, Linux: the wireless flag in /sys/class/net)
#   3) on failure, show the candidate list and exit
#
# If DHCP changes the address, the publish address becomes stale too. Rerun the same command at that point.
# The script re-queries the address at run time, so calling lan/wifi again updates it
# to the new address.
set -euo pipefail
cd "$(dirname "$0")"

ENV_FILE=.env
BIND_KEY=ARTEX_BIND

die() { printf '\033[31m[x]\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok() { printf '\033[32m[+]\033[0m %s\n' "$*"; }

# Read a value from .env (if present). Why not source it as a shell script: so that a single line is extracted safely
# even if the value contains spaces or special characters.
env_get() {
  [ -f "$ENV_FILE" ] || return 1
  local v
  v=$(grep -E "^[[:space:]]*$1=" "$ENV_FILE" 2>/dev/null | tail -n1 | cut -d= -f2-)
  [ -n "${v:-}" ] || return 1
  printf '%s' "$v"
}

# Query the IPv4 address of an interface. macOS uses ipconfig, Linux uses ip.
iface_ip() {
  local dev="$1" ip=""
  if command -v ipconfig >/dev/null 2>&1; then
    ip=$(ipconfig getifaddr "$dev" 2>/dev/null || true)
  fi
  if [ -z "$ip" ] && command -v ip >/dev/null 2>&1; then
    ip=$(ip -4 -o addr show dev "$dev" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n1)
  fi
  [ -n "$ip" ] || return 1
  printf '%s' "$ip"
}

# automatic detection: Wi-Fi interface name.
detect_wifi() {
  local dev=""
  if command -v networksetup >/dev/null 2>&1; then
    dev=$(networksetup -listallhardwareports 2>/dev/null \
      | awk '/Hardware Port: (Wi-Fi|AirPort)/{getline; print $2; exit}')
  fi
  if [ -z "$dev" ] && [ -d /sys/class/net ]; then
    for d in /sys/class/net/*; do
      [ -e "$d/wireless" ] || continue
      dev=$(basename "$d")
      break
    done
  fi
  [ -n "$dev" ] || return 1
  printf '%s' "$dev"
}

# automatic detection: LAN (wired Ethernet) interface name. If the default route is wired, use that; otherwise
# use the first Ethernet hardware port that has an IPv4 address.
detect_lan() {
  local dev=""
  if command -v route >/dev/null 2>&1; then
    dev=$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}')
    # If the default route is Wi-Fi, fall through to the wired candidates.
    local wifi; wifi=$(detect_wifi || true)
    [ "$dev" = "$wifi" ] && dev=""
  fi
  if [ -z "$dev" ] && command -v networksetup >/dev/null 2>&1; then
    while read -r d; do
      [ -n "$d" ] || continue
      if iface_ip "$d" >/dev/null 2>&1; then dev="$d"; break; fi
    done < <(networksetup -listallhardwareports 2>/dev/null \
      | awk '/Hardware Port: (Ethernet|USB .*LAN)/{getline; print $2}')
  fi
  if [ -z "$dev" ] && [ -d /sys/class/net ]; then
    for d in /sys/class/net/*; do
      [ -e "$d/wireless" ] && continue
      case "$(basename "$d")" in lo|docker*|br-*|veth*|tun*|tap*) continue ;; esac
      if iface_ip "$(basename "$d")" >/dev/null 2>&1; then dev=$(basename "$d"); break; fi
    done
  fi
  [ -n "$dev" ] || return 1
  printf '%s' "$dev"
}

# Print the candidate list in a human-readable form.
list_ifaces() {
  local found=0
  if command -v ipconfig >/dev/null 2>&1; then
    while read -r d; do
      [ -n "$d" ] || continue
      local ip; ip=$(iface_ip "$d" 2>/dev/null || true)
      [ -n "$ip" ] || continue
      printf '  %-6s %s\n' "$d" "$ip"
      found=1
    done < <(networksetup -listallhardwareports 2>/dev/null \
      | awk '/Device:/{print $2}' | sort -u)
  fi
  if [ "$found" -eq 0 ] && command -v ip >/dev/null 2>&1; then
    ip -4 -o addr show 2>/dev/null | awk '$2!="lo"{printf "  %-6s %s\n",$2,$4}' | cut -d/ -f1
  fi
  [ "$found" -eq 1 ] || return 1
}

# name → IPv4. If the name is already an IPv4 address, return it as-is.
resolve() {
  local want="$1" dev ip
  case "$want" in
    local) printf '127.0.0.1'; return 0 ;;
    any) printf '0.0.0.0'; return 0 ;;
    lan)
      dev=$(env_get ARTEX_LAN_IFACE || detect_lan || true)
      ;;
    wifi)
      dev=$(env_get ARTEX_WIFI_IFACE || detect_wifi || true)
      ;;
    *)
      dev="$want"
      ;;
  esac
  if [ -z "${dev:-}" ]; then
    printf '\033[31m[x]\033[0m Could not find the %s interface. Candidates:\n' "$want" >&2
    list_ifaces >&2 || true
    printf '  Specify an interface name or IPv4 address directly.\n' >&2
    return 1
  fi
  # Is it already in IPv4 form?
  if printf '%s' "$dev" | grep -qE '^([0-9]{1,3}\.){3}[0-9]{1,3}$'; then
    printf '%s' "$dev"
    return 0
  fi
  ip=$(iface_ip "$dev") || {
    printf '\033[31m[x]\033[0m %s has no IPv4 address.\n' "$dev" >&2
    return 1
  }
  printf '%s' "$ip"
}

set_bind() {
  local addr="$1"
  [ -f "$ENV_FILE" ] || die "$ENV_FILE is missing. Run cp .env.example .env first."
  # Replace the existing ARTEX_BIND line, or add it if absent.
  if grep -qE "^[[:space:]]*$BIND_KEY=" "$ENV_FILE"; then
    local tmp; tmp=$(mktemp "$ENV_FILE.XXXXXX")
    awk -v k="$BIND_KEY" -v v="$addr" '
      $0 ~ "^[[:space:]]*" k "=" { print k "=" v; next }
      { print }
    ' "$ENV_FILE" > "$tmp"
    mv "$tmp" "$ENV_FILE"
  else
    printf '%s=%s\n' "$BIND_KEY" "$addr" >> "$ENV_FILE"
  fi
}

recreate() {
  info "Recreating the artex container with the new binding (running work is interrupted)"
  docker compose up -d --force-recreate artex >/dev/null
  # Wait briefly for the port publishing to take effect, then check whether it actually responds.
  local addr; addr=$(env_get "$BIND_KEY" || true)
  sleep 8
  if curl -fsS -m 5 -o /dev/null "http://$addr:8787/api/health"; then
    ok "Responding at http://$addr:8787"
  else
    # 0.0.0.0 cannot be called directly as an address, so check via one of the published interfaces.
    local probe="$addr"
    [ "$addr" = "0.0.0.0" ] && probe=$(list_ifaces | awk 'NR==1{print $2}')
    if [ -n "$probe" ] && curl -fsS -m 5 -o /dev/null "http://$probe:8787/api/health"; then
      ok "Responding at http://$probe:8787 (published on all interfaces)"
    else
      printf '\033[33m[!]\033[0m Not yet responding at %s. Check the logs: docker compose logs artex\n' "$addr" >&2
    fi
  fi
}

main() {
  local arg="${1:-}"
  case "$arg" in
    ""|-h|--help|help)
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      return 0
      ;;
    list)
      list_ifaces || die "Failed to list interfaces."
      return 0
      ;;
    status)
      local cur; cur=$(env_get "$BIND_KEY" || echo "(not set → default 127.0.0.1)")
      printf 'Current %s=%s\n' "$BIND_KEY" "$cur"
      docker compose ps artex 2>/dev/null | tail -n +1 || true
      return 0
      ;;
  esac

  local addr
  addr=$(resolve "$arg") || exit 1
  info "$arg → $addr"
  set_bind "$addr"
  ok "Wrote $BIND_KEY=$addr to $ENV_FILE"
  recreate
}

main "$@"
