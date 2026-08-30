#!/bin/bash
# Remote MASQUE server install. Env: MASQUE_PORT MASQUE_HOST [MASQUE_BIN]
set -euo pipefail
: "${MASQUE_PORT:?}"
: "${MASQUE_HOST:?}"
MASQUE_BIN="${MASQUE_BIN:-/tmp/masque-vpn-server-bin}"
GEN_CONFIG="${GEN_CONFIG:-/tmp/masque-gen-config.sh}"

if [[ ! "$MASQUE_PORT" =~ ^[1-9][0-9]{0,4}$ ]] || (( MASQUE_PORT > 65535 )); then
  echo "ERROR: invalid MASQUE_PORT" >&2
  exit 1
fi

if systemctl is-active --quiet masque 2>/dev/null; then
  echo "ERROR: masque.service is already running. Stop it before reinstalling." >&2
  exit 1
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

bash "$GEN_CONFIG" --host "$MASQUE_HOST" --port "$MASQUE_PORT" --out /opt/masque/generated --clients 1 --android-only

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
