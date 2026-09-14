#!/bin/bash
# MASQUE VPN server install (systemd + certs + NAT).
# Required env: MASQUE_PORT MASQUE_HOST
# Optional: MASQUE_BIN GEN_CONFIG MASQUE_RELEASE MASQUE_REPO
# If MASQUE_BIN / GEN_CONFIG are not already on disk (masque-setup.exe uploads
# them), the script downloads vpn-server-linux-* and gen-config.sh from the
# GitHub Release and checks SHA256SUMS.
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: run as root" >&2
  exit 1
fi

: "${MASQUE_PORT:?set MASQUE_PORT (UDP listen port)}"
: "${MASQUE_HOST:?set MASQUE_HOST (public hostname or IP)}"

MASQUE_RELEASE="${MASQUE_RELEASE:-v1.5.6}"
MASQUE_REPO="${MASQUE_REPO:-Next1971/masque-vpn}"

if [[ ! "$MASQUE_PORT" =~ ^[1-9][0-9]{0,4}$ ]] || (( MASQUE_PORT > 65535 )); then
  echo "ERROR: invalid MASQUE_PORT" >&2
  exit 1
fi

if systemctl is-active --quiet masque 2>/dev/null; then
  echo "ERROR: masque.service is already running. Stop it before reinstalling." >&2
  exit 1
fi

verify_sha256() {
  local file="$1"
  local sums="$2"
  local base want got
  base=$(basename "$file")
  want=$(awk -v b="$base" '$2==b || $2=="*"b {print $1; exit}' "$sums")
  if [[ -z "$want" ]]; then
    echo "ERROR: $base not listed in SHA256SUMS" >&2
    exit 1
  fi
  got=$(sha256sum "$file" | awk '{print $1}')
  if [[ "$want" != "$got" ]]; then
    echo "ERROR: SHA256 mismatch for $base" >&2
    exit 1
  fi
}

fetch_release_files() {
  local arch bin_name url work
  arch=$(uname -m)
  case "$arch" in
    x86_64) bin_name=vpn-server-linux-amd64 ;;
    aarch64|arm64) bin_name=vpn-server-linux-arm64 ;;
    *)
      echo "ERROR: unsupported architecture: $arch" >&2
      exit 1
      ;;
  esac
  url="https://github.com/${MASQUE_REPO}/releases/download/${MASQUE_RELEASE}"
  work=$(mktemp -d /tmp/masque-release.XXXXXX)
  export DEBIAN_FRONTEND=noninteractive
  if ! command -v curl >/dev/null 2>&1; then
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates
  fi
  echo "Downloading ${MASQUE_RELEASE} (${bin_name})…"
  curl -fsSL -o "$work/SHA256SUMS" "$url/SHA256SUMS"
  curl -fsSL -o "$work/$bin_name" "$url/$bin_name"
  curl -fsSL -o "$work/gen-config.sh" "$url/gen-config.sh"
  verify_sha256 "$work/$bin_name" "$work/SHA256SUMS"
  verify_sha256 "$work/gen-config.sh" "$work/SHA256SUMS"
  chmod 0755 "$work/$bin_name" "$work/gen-config.sh"
  MASQUE_BIN="$work/$bin_name"
  GEN_CONFIG="$work/gen-config.sh"
}

if [[ -n "${MASQUE_BIN:-}" && -f "$MASQUE_BIN" && -n "${GEN_CONFIG:-}" && -f "$GEN_CONFIG" ]]; then
  :
else
  fetch_release_files
fi

if [[ ! -x "$MASQUE_BIN" ]] && [[ ! -f "$MASQUE_BIN" ]]; then
  echo "ERROR: server binary missing at $MASQUE_BIN" >&2
  exit 1
fi
if [[ ! -f "$GEN_CONFIG" ]]; then
  echo "ERROR: gen-config.sh missing at $GEN_CONFIG" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq openssl iptables iproute2

WAN_IF=$(ip -4 route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit}}')
if [[ -z "$WAN_IF" ]]; then
  echo "ERROR: cannot detect WAN interface (no default IPv4 route)" >&2
  exit 1
fi
if [[ ! "$WAN_IF" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "ERROR: unexpected WAN interface name: $WAN_IF" >&2
  exit 1
fi

install -d -m 0755 /opt/masque/cert /opt/masque/ca
install -m 0755 "$MASQUE_BIN" /opt/masque/vpn-server

GEN_ARGS=(--host "$MASQUE_HOST" --port "$MASQUE_PORT" --out /opt/masque/generated --clients 1 --android-only)
if [[ -n "${MASQUE_IP:-}" ]]; then
  GEN_ARGS+=(--ip "$MASQUE_IP")
fi
if [[ -n "${MASQUE_DIAL:-}" ]]; then
  GEN_ARGS+=(--dial "$MASQUE_DIAL")
fi
bash "$GEN_CONFIG" "${GEN_ARGS[@]}"

install -d -m 0755 /opt/masque/state /opt/masque/clients
# App issuance starts at masque-client-9 so test CN 1–8 (and the bootstrap
# unnumbered masque-client from this install) stay untouched.
if [[ ! -f /opt/masque/state/next_index ]]; then
  echo 9 > /opt/masque/state/next_index
fi
install -m 0755 "$GEN_CONFIG" /opt/masque/gen-config.sh

install -m 0644 /opt/masque/generated/server/config.server.toml /opt/masque/config.server.toml
install -m 0644 /opt/masque/generated/server/server.crt /opt/masque/cert/server.crt
install -m 0600 /opt/masque/generated/server/server.key /opt/masque/cert/server.key
install -m 0644 /opt/masque/generated/server/ca.crt /opt/masque/cert/ca.crt
install -m 0644 /opt/masque/generated/ca/ca.crt /opt/masque/ca/ca.crt
install -m 0600 /opt/masque/generated/ca/ca.key /opt/masque/ca/ca.key

cat > /etc/sysctl.d/99-masque-udp.conf << 'EOF'
net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
EOF
sysctl --system >/dev/null 2>&1 || sysctl -p /etc/sysctl.d/99-masque-udp.conf >/dev/null

cat > /etc/systemd/system/masque.service << EOF
[Unit]
Description=MASQUE VPN server (CONNECT-IP/QUIC, mTLS)
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
WorkingDirectory=/opt/masque
ExecStartPre=/sbin/sysctl -w net.ipv4.ip_forward=1
ExecStartPre=/sbin/sysctl -w net.ipv6.conf.all.forwarding=1
ExecStartPre=/sbin/sysctl -w net.core.rmem_max=67108864
ExecStartPre=/sbin/sysctl -w net.core.wmem_max=67108864
ExecStartPre=/bin/sh -c 'iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -o ${WAN_IF} -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -o ${WAN_IF} -j MASQUERADE'
ExecStartPre=/bin/sh -c 'ip6tables -t nat -C POSTROUTING -s fd00:8::/64 -o ${WAN_IF} -j MASQUERADE 2>/dev/null || ip6tables -t nat -A POSTROUTING -s fd00:8::/64 -o ${WAN_IF} -j MASQUERADE || true'
ExecStartPre=/bin/sh -c 'iptables -C FORWARD -i masque0 -o ${WAN_IF} -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -i masque0 -o ${WAN_IF} -j ACCEPT'
ExecStartPre=/bin/sh -c 'iptables -C FORWARD -i ${WAN_IF} -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -I FORWARD 2 -i ${WAN_IF} -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT'
ExecStartPre=/bin/sh -c 'ip6tables -C FORWARD -i masque0 -o ${WAN_IF} -j ACCEPT 2>/dev/null || ip6tables -I FORWARD 1 -i masque0 -o ${WAN_IF} -j ACCEPT || true'
ExecStartPre=/bin/sh -c 'ip6tables -C FORWARD -i ${WAN_IF} -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || ip6tables -I FORWARD 2 -i ${WAN_IF} -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT || true'
ExecStart=/opt/masque/vpn-server -config /opt/masque/config.server.toml
User=root
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
DeviceAllow=/dev/net/tun rw
Restart=on-failure
RestartSec=3
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

if command -v ufw >/dev/null 2>&1; then
  ufw allow "${MASQUE_PORT}/udp" comment 'MASQUE VPN' || true
  if ufw status 2>/dev/null | grep -qi 'Status: active'; then
    ufw reload || true
  fi
  echo "UFW: allowed UDP ${MASQUE_PORT}"
else
  echo "UFW: not installed (skipped)"
fi

systemctl daemon-reload
systemctl enable --now masque.service
sleep 2
if ! systemctl is-active --quiet masque; then
  journalctl -u masque -n 40 --no-pager >&2 || true
  echo "ERROR: masque.service failed to start" >&2
  exit 1
fi

echo "WAN_IF=${WAN_IF}"
echo "LISTEN_OK"
ss -H -uln | grep -E ":${MASQUE_PORT}([[:space:]]|$)" || ss -ulnp | grep -E ":${MASQUE_PORT}\\b" || true
