v1.5.6 is a full GitHub release of **QUIC to the server over IPv6**. It does **not** replace **Latest**. Daily driver stays **[v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3)**.

**Previous 1.5.x clients and servers stay compatible over IPv4. If your VPN already works, you do not need to install 1.5.6.**

## What this tag is

- QUIC over IPv6: A + AAAA, IPv6 preferred; optional `gen-config.sh --dial` writes `[IPv6]:port` into client profiles.
- Server bind defaults to `[::]:port` (dual-stack when `net.ipv6.bindv6only=0`). `install.sh` accepts `MASQUE_IP` and `MASQUE_DIAL`.
- Android `1.5.6` (`versionCode` 22); Windows product **1.5.6**.
- Windows **Disconnect** closes the QUIC session (pre-1.5.6 clients stay Connected until traffic or `sc stop MasqueVpn`).
- `install.sh`, `gen-config.sh`, `SHA256SUMS` (the v1.5.5 packaging slice that was not published as its own tag).
- New `vpn-server-linux-amd64` / `vpn-server-linux-arm64`.

## Who should update

- Operators whose VPS has WAN IPv6 and who want the client path on UDP/443 IPv6.
- Anyone who wants the Windows Disconnect fix.

Everyone else: stay on **v1.5.3** (Latest). Stay on **v1.5.4** only if you already use `alt_port` and do not need IPv6 QUIC.

## Compatibility

All earlier MASQUE **1.5.x** (and the v1.3+/v1.4.x CONNECT-IP line) keep working:

- Old **v1.5.3 / v1.5.4** clients reach a **v1.5.6** server over **IPv4** (same RFC 9484 capsules; keep the IPv4 SAN).
- A **v1.5.6** client reaches an older IPv4-only server when the profile is IPv4 (or a name without a real AAAA).
- Do not publish AAAA for a hostname still used by pre-1.5.6 clients (`ResolveUDPAddr` can pick IPv6 while the old socket is IPv4-only).
- Dual-port (`alt_port`) is unchanged from v1.5.4.

## `masque-setup.exe`

Experimental. One UDP listen port. Public host is IPv4 or DNS only (no IPv6 literal in the wizard). The 1.5.6 server it installs already listens dual-stack. Second port: `gen-config.sh --alt-port`. QUIC to a specific IPv6: `MASQUE_DIAL` / `MASQUE_IP` on `install.sh`, not the wizard.

## iOS

No iOS build is attached. TestFlight stays **v1.7**.

## Notes

- Experimental, self-hosted software. No third-party security audit.
- DoH/DoT is not in this tag.
- The AGP 9.4 / Gradle 9.6 bump did not land; Android stays on the 1.5.4 toolchain.

## Files

- `masque-1.5.6.msi` — Windows VPN client (Disconnect closes QUIC)
- `masque-setup.exe` — experimental VPS installer
- `masque-phone-1.5.6.apk` — Android phone
- `masque-tv-1.5.6.apk` — Android TV
- `vpn-server-linux-amd64` / `vpn-server-linux-arm64` — server
- `install.sh` / `gen-config.sh` — VPS install
- `SHA256SUMS`
