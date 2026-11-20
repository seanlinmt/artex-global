#!/usr/bin/env bash
# ARTEX 바인딩 선택 스크립트.
#
# 기본 docker-compose.yml 은 127.0.0.1 에만 게시합니다(원격 노출 방지). 이 스크립트는
# 그 게시 주소를 "지금 고른 인터페이스 하나"로 바꿉니다. 즉 LAN 이나 Wi-Fi 중 하나만
# 노출하고, 나머지 인터페이스로는 들어오지 않습니다.
#
# 사용법:
#   ./bind.sh list              사용 가능한 인터페이스와 IPv4 주소를 보여줍니다
#   ./bind.sh local             127.0.0.1 로 되돌립니다(기본값, 로컬 전용)
#   ./bind.sh lan               LAN 인터페이스 주소로 게시합니다
#   ./bind.sh wifi              Wi-Fi 인터페이스 주소로 게시합니다
#   ./bind.sh any               모든 인터페이스(0.0.0.0)로 게시합니다
#   ./bind.sh en12              인터페이스 이름을 직접 지정합니다
#   ./bind.sh 192.168.20.22     IPv4 주소를 직접 지정합니다
#   ./bind.sh status            현재 바인딩과 게시 상태를 보여줍니다
#
# lan/wifi 는 다음 순서로 대상을 찾습니다:
#   1) 환경변수 ARTEX_LAN_IFACE / ARTEX_WIFI_IFACE (또는 .env 의 같은 이름)
#   2) 자동 감지(macOS: networksetup, Linux: /sys/class/net 의 wireless 플래그)
#   3) 실패 시 후보 목록을 보여 주고 종료
#
# DHCP 로 주소가 바뀌면 게시 주소도 낡습니다. 그때는 같은 명령을 다시 실행하십시오.
# 스크립트가 실행 시점에 주소를 다시 조회하므로, lan/wifi 를 다시 부르면 새 주소로
# 갱신됩니다.
set -euo pipefail
cd "$(dirname "$0")"

ENV_FILE=.env
BIND_KEY=ARTEX_BIND

die() { printf '\033[31m[x]\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok() { printf '\033[32m[+]\033[0m %s\n' "$*"; }

# .env 에서 값을 읽습니다(있으면). 셸로 source 하지 않는 이유: 값에 공백·특수문자가
# 들어가도 안전하게 한 줄만 뽑기 위해서입니다.
env_get() {
  [ -f "$ENV_FILE" ] || return 1
  local v
  v=$(grep -E "^[[:space:]]*$1=" "$ENV_FILE" 2>/dev/null | tail -n1 | cut -d= -f2-)
  [ -n "${v:-}" ] || return 1
  printf '%s' "$v"
}

# 인터페이스의 IPv4 주소를 조회합니다. macOS 는 ipconfig, Linux 는 ip 를 씁니다.
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

# 자동 감지: Wi-Fi 인터페이스 이름.
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

# 자동 감지: LAN(유선 이더넷) 인터페이스 이름. 기본 라우트가 유선이면 그것을, 아니면
# 이더넷 하드웨어 포트 중 IPv4 가 있는 첫 장치를 씁니다.
detect_lan() {
  local dev=""
  if command -v route >/dev/null 2>&1; then
    dev=$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}')
    # 기본 라우트가 Wi-Fi 면 유선 후보로 넘어갑니다.
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

# 후보 목록을 사람이 읽을 수 있게 출력합니다.
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

# 이름 → IPv4. 이름이 이미 IPv4 면 그대로 돌려줍니다.
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
    printf '\033[31m[x]\033[0m %s 인터페이스를 찾지 못했습니다. 후보:\n' "$want" >&2
    list_ifaces >&2 || true
    printf '  인터페이스 이름이나 IPv4 주소를 직접 지정하십시오.\n' >&2
    return 1
  fi
  # 이미 IPv4 형태인가?
  if printf '%s' "$dev" | grep -qE '^([0-9]{1,3}\.){3}[0-9]{1,3}$'; then
    printf '%s' "$dev"
    return 0
  fi
  ip=$(iface_ip "$dev") || {
    printf '\033[31m[x]\033[0m %s 에 IPv4 주소가 없습니다.\n' "$dev" >&2
    return 1
  }
  printf '%s' "$ip"
}

set_bind() {
  local addr="$1"
  [ -f "$ENV_FILE" ] || die "$ENV_FILE 이 없습니다. 먼저 cp .env.example .env 하십시오."
  # 기존 ARTEX_BIND 줄을 교체하고, 없으면 추가합니다.
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
  info "artex 컨테이너를 새 바인딩으로 다시 만듭니다(실행 중인 작업은 중단됩니다)"
  docker compose up -d --force-recreate artex >/dev/null
  # 포트 게시가 반영될 때까지 잠깐 기다린 뒤 실제로 응답하는지 확인합니다.
  local addr; addr=$(env_get "$BIND_KEY" || true)
  sleep 8
  if curl -fsS -m 5 -o /dev/null "http://$addr:8787/api/health"; then
    ok "http://$addr:8787 에서 응답합니다"
  else
    # 0.0.0.0 은 주소로 직접 부를 수 없으므로, 게시된 인터페이스 중 하나로 확인합니다.
    local probe="$addr"
    [ "$addr" = "0.0.0.0" ] && probe=$(list_ifaces | awk 'NR==1{print $2}')
    if [ -n "$probe" ] && curl -fsS -m 5 -o /dev/null "http://$probe:8787/api/health"; then
      ok "http://$probe:8787 에서 응답합니다(모든 인터페이스 게시)"
    else
      printf '\033[33m[!]\033[0m %s 에서 아직 응답하지 않습니다. 로그를 확인하십시오: docker compose logs artex\n' "$addr" >&2
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
      list_ifaces || die "인터페이스를 나열하지 못했습니다."
      return 0
      ;;
    status)
      local cur; cur=$(env_get "$BIND_KEY" || echo "(설정 없음 → 기본값 127.0.0.1)")
      printf '현재 %s=%s\n' "$BIND_KEY" "$cur"
      docker compose ps artex 2>/dev/null | tail -n +1 || true
      return 0
      ;;
  esac

  local addr
  addr=$(resolve "$arg") || exit 1
  info "$arg → $addr"
  set_bind "$addr"
  ok "$ENV_FILE 의 $BIND_KEY=$addr 로 기록했습니다"
  recreate
}

main "$@"
