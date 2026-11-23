#!/usr/bin/env bash
# ARTEX egress route selection script (macOS).
#
# Replacing the default route wholesale with "a single gateway" either cuts off the LAN, or conversely overwrites
# the LAN default route so the original path disappears. So this script leaves the default route untouched and
# covers all of IPv4 with two pieces: 0.0.0.0/1 + 128.0.0.0/1. By longest-prefix match these two pieces always take
# precedence over the existing default, so traffic leaves via the specified gateway while the LAN subnet's
# on-link routes (connected routes) remain intact.
#
# Usage:
#   ./route.sh wifi      Send all traffic (except LAN subnets) to the Wi-Fi gateway
#   ./route.sh lan       Delete the additional routes and return to the default (wired LAN)
#   ./route.sh status    Show the current additional routes and the actual per-destination routes
#
# How it works:
#   - Reads the Wi-Fi service name and gateway from DHCP (no hardcoded values).
#   - Saves via networksetup -setadditionalroutes, so it persists across reboots.
#   - Preserves any other additional routes already on this service and replaces only our two pieces.
#   - Does not need root (verified). Retries with sudo -n if needed.
#
# Cautions:
#   - This route hard-references the Wi-Fi gateway. If Wi-Fi drops, those pieces become
#     blackholes, and macOS does not automatically revert to the LAN default.
#     While on the move, revert with ./route.sh lan.
#   - In environments where both interfaces NAT to the same public IP, even when leaving via Wi-Fi the peer sees
#     the same source IP as the LAN. Only the actual path differs.
set -euo pipefail

WIFI_SERVICE_DEFAULT="Wi-Fi"
SPLIT_DEST_A="0.0.0.0"
SPLIT_DEST_B="128.0.0.0"
SPLIT_MASK="128.0.0.0"

die() { printf '\033[31m[x]\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok() { printf '\033[32m[+]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[!]\033[0m %s\n' "$*" >&2; }

need_macos() {
  [ "$(uname -s)" = "Darwin" ] || die "This script is macOS-only (uses networksetup)."
  command -v networksetup >/dev/null 2>&1 || die "networksetup not found."
}

# Find the Wi-Fi service name (if the default "Wi-Fi" is absent, trace it back via hardware ports).
wifi_service() {
  local services svc
  services=$(networksetup -listallnetworkservices 2>/dev/null | tail -n +2 | sed 's/^\*//')
  while IFS= read -r svc; do
    [ -n "$svc" ] || continue
    local dev
    dev=$(networksetup -listnetworkserviceorder 2>/dev/null \
      | awk -v s="$svc" '
          /^\([0-9]+\)/ { name=substr($0, index($0,")")+2); next }
          /Device:/ && name==s { gsub(/.*Device: /,""); gsub(/\)/,""); print; exit }')
    [ -n "$dev" ] || continue
    local port
    port=$(networksetup -listallhardwareports 2>/dev/null \
      | awk -v d="$dev" '/Hardware Port:/{hp=substr($0,index($0,":")+2)} /Device:/{gsub(/.*Device: /,""); if($0==d) print hp}')
    case "$port" in
      Wi-Fi|AirPort) printf '%s' "$svc"; return 0 ;;
    esac
  done <<< "$services"
  return 1
}

# The service's device name (e.g. en0).
service_device() {
  networksetup -listnetworkserviceorder 2>/dev/null \
    | awk -v s="$1" '
        /^\([0-9]+\)/ { name=substr($0, index($0,")")+2); next }
        /Device:/ && name==s { gsub(/.*Device: /,""); gsub(/\)/,""); print; exit }'
}

# The device's DHCP router.
device_router() {
  ipconfig getoption "$1" router 2>/dev/null || true
}

# The device's IPv4 address.
device_ip() {
  ipconfig getifaddr "$1" 2>/dev/null || true
}

# Keep only the service's existing additional routes that are not our two pieces, printing a "dest mask gw" list.
other_routes() {
  local svc="$1" gw="$2"
  networksetup -getadditionalroutes "$svc" 2>/dev/null | while read -r d m g; do
    [ -n "${d:-}" ] || continue
    case "$d" in
      "There are no additional IPv4 routes"*) continue ;;
    esac
    # Skip the two pieces we manage regardless of gateway. Otherwise the same pieces stored on a previous Wi-Fi
    # network linger and accumulate with only a different gateway (that stale gateway becomes a blackhole on
    # the current network). Either way, both pieces are re-added below with the current gateway, so dropping
    # the stale copies here is safe.
    if [ "$m" = "$SPLIT_MASK" ] \
       && { [ "$d" = "$SPLIT_DEST_A" ] || [ "$d" = "$SPLIT_DEST_B" ]; }; then
      continue
    fi
    printf '%s %s %s\n' "$d" "$m" "$g"
  done
}

# networksetup call. Retries with sudo -n if root is required.
ns_set() {
  if networksetup -setadditionalroutes "$@" 2>/dev/null; then
    return 0
  fi
  if sudo -n networksetup -setadditionalroutes "$@" 2>/dev/null; then
    return 0
  fi
  return 1
}

apply_wifi() {
  local svc dev gw ip
  svc=$(wifi_service) || die "Could not find the Wi-Fi service."
  dev=$(service_device "$svc")
  [ -n "$dev" ] || die "Could not find the device name for $svc."
  ip=$(device_ip "$dev")
  [ -n "$ip" ] || die "$dev has no IPv4 address (check the Wi-Fi connection)."
  gw=$(device_router "$dev")
  [ -n "$gw" ] || die "Could not read the DHCP router for $dev."

  info "Wi-Fi service=$svc device=$dev address=$ip gateway=$gw"

  # First verify that the gateway actually responds. Using an unresponsive gateway as the default
  # would blackhole all subsequent traffic.
  if ! ping -c1 -W2 -S "$ip" "$gw" >/dev/null 2>&1; then
    warn "Gateway $gw is not responding. Setting it anyway (it may apply once connected)."
  fi

  # Preserve existing unrelated additional routes, then append our two pieces.
  local -a args=()
  while read -r d m g; do
    [ -n "${d:-}" ] || continue
    args+=("$d" "$m" "$g")
  done < <(other_routes "$svc" "$gw")
  args+=("$SPLIT_DEST_A" "$SPLIT_MASK" "$gw" "$SPLIT_DEST_B" "$SPLIT_MASK" "$gw")

  if ns_set "$svc" "${args[@]}"; then
    ok "Saved additional routes on $svc (persists across reboots)"
  else
    die "networksetup -setadditionalroutes failed (check permissions)."
  fi

  # Verify the applied result.
  sleep 1
  local got; got=$(route -n get 1.1.1.1 2>/dev/null | awk '/gateway:/{print $2} /interface:/{print $2}' | paste -sd' ' -)
  info "1.1.1.1 route: $got (expected: gw=$gw)"
  case "$got" in
    "$gw"*) ok "Confirmed traffic is leaving via Wi-Fi." ;;
    *) warn "Still not leaving via Wi-Fi: $got" ;;
  esac
}

clear_routes() {
  local svc
  svc=$(wifi_service) || die "Could not find the Wi-Fi service."
  if ns_set "$svc"; then
    ok "Deleted the additional routes on $svc (returned to the default route)"
  else
    die "Failed to delete additional routes."
  fi
}

status() {
  local svc dev gw
  svc=$(wifi_service) || svc="$WIFI_SERVICE_DEFAULT"
  dev=$(service_device "$svc" 2>/dev/null || true)
  gw=$(device_router "${dev:-en0}" 2>/dev/null || true)

  printf 'Service: %s (device %s, gateway %s)\n' "$svc" "${dev:-?}" "${gw:-?}"
  echo "Additional routes:"
  networksetup -getadditionalroutes "$svc" 2>/dev/null | sed 's/^/  /'
  echo "Per-destination routes:"
  local t
  for t in 192.168.20.1 192.168.20.12 1.1.1.1 8.8.8.8; do
    printf '  %-14s ' "$t"
    route -n get "$t" 2>/dev/null | awk '/gateway:/{g=$2} /interface:/{i=$2} END{if(g)printf "gw=%s ",g; printf "if=%s\n",i}'
  done
  echo "Default route:"
  netstat -rn -f inet 2>/dev/null | awk '$1=="default"{printf "  %-18s %-18s %s\n",$1,$2,$4}'
}

main() {
  need_macos
  case "${1:-}" in
    wifi) apply_wifi ;;
    lan|default|clear) clear_routes ;;
    status) status ;;
    ""|-h|--help|help)
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      ;;
    *) die "Unknown argument: $1 (wifi|lan|status)" ;;
  esac
}

main "$@"
