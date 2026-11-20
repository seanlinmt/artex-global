#!/usr/bin/env bash
# ARTEX egress 경로 선택 스크립트 (macOS).
#
# 기본 라우트를 "하나의 게이트웨이"로 통째로 바꾸면 LAN 이 끊기거나, 반대로 LAN 기본
# 라우트를 덮어써서 원래 경로가 사라집니다. 그래서 여기서는 기본 라우트를 건드리지 않고
# 0.0.0.0/1 + 128.0.0.0/1 두 조각으로 IPv4 전체를 덮습니다. 이 두 조각은 longest-prefix
# match 로 기존 default 보다 항상 우선하므로, 지정한 게이트웨이로 나가면서도 LAN 서브넷의
# on-link 경로(connected route)는 그대로 남습니다.
#
# 사용법:
#   ./route.sh wifi      모든 트래픽(단, LAN 서브넷 제외)을 Wi-Fi 게이트웨이로 보냅니다
#   ./route.sh lan       추가 라우트를 지워 기본값(유선 LAN)으로 되돌립니다
#   ./route.sh status    현재 추가 라우트와 목적지별 실제 경로를 보여줍니다
#
# 동작 방식:
#   - Wi-Fi 서비스 이름·게이트웨이는 DHCP 에서 읽습니다(고정값을 쓰지 않음).
#   - networksetup -setadditionalroutes 로 저장하므로 재부팅 후에도 유지됩니다.
#   - 이 서비스에 이미 있던 다른 추가 라우트는 보존하고, 우리 두 조각만 교체합니다.
#   - root 가 필요 없습니다(확인 완료). 필요하면 sudo -n 으로 재시도합니다.
#
# 주의:
#   - 이 경로는 Wi-Fi 게이트웨이를 고정 참조합니다. Wi-Fi 가 끊기면 해당 조각들은
#     블랙홀이 되고, macOS 가 자동으로 LAN default 로 되돌리지 않습니다.
#     이동 중에는 ./route.sh lan 으로 되돌리십시오.
#   - 두 인터페이스가 같은 공인 IP 로 NAT 되는 환경에서는, Wi-Fi 로 나가도 상대가 보는
#     출발지 IP 는 LAN 과 동일합니다. 실제 경로만 다릅니다.
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
  [ "$(uname -s)" = "Darwin" ] || die "이 스크립트는 macOS 전용입니다(networksetup 사용)."
  command -v networksetup >/dev/null 2>&1 || die "networksetup 을 찾을 수 없습니다."
}

# Wi-Fi 서비스 이름을 찾습니다(기본값 "Wi-Fi" 가 없으면 하드웨어 포트로 역추적).
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

# 서비스의 장치 이름(en0 등).
service_device() {
  networksetup -listnetworkserviceorder 2>/dev/null \
    | awk -v s="$1" '
        /^\([0-9]+\)/ { name=substr($0, index($0,")")+2); next }
        /Device:/ && name==s { gsub(/.*Device: /,""); gsub(/\)/,""); print; exit }'
}

# 장치의 DHCP 라우터.
device_router() {
  ipconfig getoption "$1" router 2>/dev/null || true
}

# 장치의 IPv4 주소.
device_ip() {
  ipconfig getifaddr "$1" 2>/dev/null || true
}

# 서비스의 기존 추가 라우트 중 우리 두 조각이 아닌 것만 남겨 "dest mask gw" 목록으로 출력.
other_routes() {
  local svc="$1" gw="$2"
  networksetup -getadditionalroutes "$svc" 2>/dev/null | while read -r d m g; do
    [ -n "${d:-}" ] || continue
    case "$d" in
      "There are no additional IPv4 routes"*) continue ;;
    esac
    # 우리가 넣은 두 조각(대상·마스크·게이트웨이 동일)은 건너뜁니다.
    if [ "$g" = "$gw" ] && [ "$m" = "$SPLIT_MASK" ] \
       && { [ "$d" = "$SPLIT_DEST_A" ] || [ "$d" = "$SPLIT_DEST_B" ]; }; then
      continue
    fi
    printf '%s %s %s\n' "$d" "$m" "$g"
  done
}

# networksetup 호출. root 가 필요하면 sudo -n 으로 재시도합니다.
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
  svc=$(wifi_service) || die "Wi-Fi 서비스를 찾지 못했습니다."
  dev=$(service_device "$svc")
  [ -n "$dev" ] || die "$svc 의 장치 이름을 찾지 못했습니다."
  ip=$(device_ip "$dev")
  [ -n "$ip" ] || die "$dev 에 IPv4 주소가 없습니다(Wi-Fi 연결을 확인하십시오)."
  gw=$(device_router "$dev")
  [ -n "$gw" ] || die "$dev 의 DHCP 라우터를 읽지 못했습니다."

  info "Wi-Fi 서비스=$svc 장치=$dev 주소=$ip 게이트웨이=$gw"

  # 게이트웨이가 실제로 응답하는지 먼저 확인합니다. 응답하지 않는 게이트웨이를 기본으로
  # 삼으면 이후 모든 트래픽이 블랙홀이 됩니다.
  if ! ping -c1 -W2 -S "$ip" "$gw" >/dev/null 2>&1; then
    warn "게이트웨이 $gw 가 응답하지 않습니다. 그래도 설정합니다(연결 후 적용될 수 있음)."
  fi

  # 기존의 무관한 추가 라우트를 보존한 뒤 우리 두 조각을 덧붙입니다.
  local -a args=()
  while read -r d m g; do
    [ -n "${d:-}" ] || continue
    args+=("$d" "$m" "$g")
  done < <(other_routes "$svc" "$gw")
  args+=("$SPLIT_DEST_A" "$SPLIT_MASK" "$gw" "$SPLIT_DEST_B" "$SPLIT_MASK" "$gw")

  if ns_set "$svc" "${args[@]}"; then
    ok "$svc 에 추가 라우트를 저장했습니다(재부팅 후에도 유지)"
  else
    die "networksetup -setadditionalroutes 실패(권한을 확인하십시오)."
  fi

  # 적용 결과를 검증합니다.
  sleep 1
  local got; got=$(route -n get 1.1.1.1 2>/dev/null | awk '/gateway:/{print $2} /interface:/{print $2}' | paste -sd' ' -)
  info "1.1.1.1 경로: $got (기대: gw=$gw)"
  case "$got" in
    "$gw"*) ok "Wi-Fi 로 나가는 것을 확인했습니다." ;;
    *) warn "아직 Wi-Fi 로 나가지 않습니다: $got" ;;
  esac
}

clear_routes() {
  local svc
  svc=$(wifi_service) || die "Wi-Fi 서비스를 찾지 못했습니다."
  if ns_set "$svc"; then
    ok "$svc 의 추가 라우트를 지웠습니다(기본 라우트로 복귀)"
  else
    die "추가 라우트 삭제 실패."
  fi
}

status() {
  local svc dev gw
  svc=$(wifi_service) || svc="$WIFI_SERVICE_DEFAULT"
  dev=$(service_device "$svc" 2>/dev/null || true)
  gw=$(device_router "${dev:-en0}" 2>/dev/null || true)

  printf '서비스: %s (장치 %s, 게이트웨이 %s)\n' "$svc" "${dev:-?}" "${gw:-?}"
  echo "추가 라우트:"
  networksetup -getadditionalroutes "$svc" 2>/dev/null | sed 's/^/  /'
  echo "목적지별 경로:"
  local t
  for t in 192.168.20.1 192.168.20.12 1.1.1.1 8.8.8.8; do
    printf '  %-14s ' "$t"
    route -n get "$t" 2>/dev/null | awk '/gateway:/{g=$2} /interface:/{i=$2} END{if(g)printf "gw=%s ",g; printf "if=%s\n",i}'
  done
  echo "기본 라우트:"
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
    *) die "알 수 없는 인자: $1 (wifi|lan|status)" ;;
  esac
}

main "$@"
