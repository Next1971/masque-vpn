#!/bin/sh
# Bring up forwarding/NAT, then exec vpn-server.
# MASQUE_WAN_IF — public interface for MASQUERADE (default: detect from default route).
# MASQUE_POOL   — client pool CIDR for NAT (default: 10.8.0.0/24).
set -eu

WAN_IF="${MASQUE_WAN_IF:-}"
if [ -z "$WAN_IF" ]; then
  WAN_IF=$(ip -4 route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit}}')
fi
if [ -z "$WAN_IF" ]; then
  echo "entrypoint: cannot detect WAN interface; set MASQUE_WAN_IF" >&2
  exit 1
fi

POOL="${MASQUE_POOL:-10.8.0.0/24}"
POOL_V6="${MASQUE_POOL_V6:-fd00:8::/64}"

sysctl -w net.ipv4.ip_forward=1 >/dev/null
sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null 2>&1 || true
sysctl -w net.core.rmem_max=67108864 >/dev/null 2>&1 || true
sysctl -w net.core.wmem_max=67108864 >/dev/null 2>&1 || true

iptables -t nat -C POSTROUTING -s "$POOL" -o "$WAN_IF" -j MASQUERADE 2>/dev/null \
  || iptables -t nat -A POSTROUTING -s "$POOL" -o "$WAN_IF" -j MASQUERADE
ip6tables -t nat -C POSTROUTING -s "$POOL_V6" -o "$WAN_IF" -j MASQUERADE 2>/dev/null \
  || ip6tables -t nat -A POSTROUTING -s "$POOL_V6" -o "$WAN_IF" -j MASQUERADE || true
iptables -C FORWARD -i masque0 -o "$WAN_IF" -j ACCEPT 2>/dev/null \
  || iptables -I FORWARD 1 -i masque0 -o "$WAN_IF" -j ACCEPT
iptables -C FORWARD -i "$WAN_IF" -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null \
  || iptables -I FORWARD 2 -i "$WAN_IF" -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT
ip6tables -C FORWARD -i masque0 -o "$WAN_IF" -j ACCEPT 2>/dev/null \
  || ip6tables -I FORWARD 1 -i masque0 -o "$WAN_IF" -j ACCEPT || true
ip6tables -C FORWARD -i "$WAN_IF" -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null \
  || ip6tables -I FORWARD 2 -i "$WAN_IF" -o masque0 -m state --state RELATED,ESTABLISHED -j ACCEPT || true

echo "entrypoint: WAN=$WAN_IF pool=$POOL pool_v6=$POOL_V6; starting vpn-server"

exec /usr/local/bin/vpn-server -config /opt/masque/config.server.toml
