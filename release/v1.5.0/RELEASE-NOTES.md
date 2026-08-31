v1.5.0 adds **optional IPv6 inside the tunnel**. Traffic can use a ULA address (`fd00:8::/64`) and NAT66 on the VPS. IPv4-only server configs (no `tun_addr_v6` / `pool_cidr_v6`) keep working. Client profiles do not change.

Connecting to the VPN is still **IPv4 QUIC**. Do not publish an AAAA for the VPN hostname in this release (that needs a host-route bypass first).

## Highlights

- Server: optional IPv6 pool, sticky `/128` per client CN, `route_v6` (`::/0`).
- NAT66 in systemd, Docker, and `gen-config.sh`.
- Android 1.5.0 (`versionCode` 17): assigned IPv6 on TUN (no longer sunk when the server hands out v6).
- Windows GUI and console: IPv6 on Wintun plus `::/0` through the tunnel.
- Linux console `vpn-client`: same IPv6 address and default/test routes.

## Who should update

- Anyone whose VPS has WAN IPv6 and who wants AAAA destinations to work through the VPN.
- Existing clients on IPv4-only servers: optional; behavior stays IPv4-only until you add the v6 keys and replace the server binary.

## Notes

- This is experimental, self-hosted software. No third-party security audit.
- Android APKs are **signed** (sideload with unknown sources). Unsigned builds would not install on current Android.
- Linux server binaries contain **no certificates** — keep your existing CA; do not re-run `gen-config.sh` just to pick up IPv6 (add the three keys to `config.server.toml` instead).

## Files

- `masque-1.5.0.msi` — Windows
- `masque-phone-1.5.0.apk` — Android phone
- `masque-tv-1.5.0.apk` — Android TV
- `vpn-server-linux-amd64` / `vpn-server-linux-arm64` — server
