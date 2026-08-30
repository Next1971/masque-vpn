# Changelog

All notable changes to MASQUE VPN are documented here.

## [Unreleased]

## [v1.5.0] - 2026-08-30

**GitHub Latest release.** Optional **IPv6 inside the tunnel**. Existing IPv4-only server configs (no `tun_addr_v6` / `pool_cidr_v6`) behave as before. Client profiles do not change. QUIC to the server stays on IPv4; do not add an AAAA for the VPN hostname in this release.

### Added

- Server: optional ULA pool `fd00:8::/64`, sticky `/128` per client CN, `route_v6` (`::/0` by default).
- NAT66 (ip6tables MASQUERADE) in systemd, Docker entrypoint, and `gen-config.sh`.
- Android 1.5.0 (`versionCode` 17): assigned IPv6 on TUN, forward instead of sink when the server hands out v6.
- Windows GUI and console clients: IPv6 address on Wintun plus `::/0` through the tunnel.
- Linux console `vpn-client`: IPv6 address and default (or test) route through TUN.

### Changed

- Android NDK pin **29.0.13599879** (AGP 9; local and CI).
- Windows product version **1.5.0** (winres / MSI).

## [v1.4.2] - 2026-08-29

**Experimental GitHub pre-release.** This tag is a test of the Windows VPS installer (`masque-setup.exe`), not a promotion of the Android/Windows VPN clients. Clients stay on **v1.4.0** (stable) / **v1.4.1** (maintenance pre-release). The setup tool can brick a misconfigured VPS, stores SSH host keys locally, and has **no certificate revocation**. Use only on machines you can wipe.

### Added

- **Windows VPS installer** (`masque-setup.exe`): SSH as root (password or key), refuse OS other than Ubuntu 22.04/24.04 or Debian 12, suggest free UDP ports (443, 2053, 8443, 41234), open ufw if present, install the MASQUE server, then **OK / not OK** only after a QUIC probe to the live listener. The CA key stays on the VPS.
- Setup app **does not overwrite** an existing `/opt/masque` install: Connect detects the service/config and goes straight to issuing keys.
- Setup app **issues `profile.masque` from #9** (`masque-client-9` …): numbers **1–8 are reserved test CNs**. Shows app-issued count out of 253 pool slots. Windows GUI imports the same `profile.masque` (no separate toml bundle).
- `gen-config.sh`: `--index`, `--android-only`, `--client-only` so later clients do not replace `server.crt`.

### Requirements (setup EXE)

Put the matching **Linux server binary next to** `masque-setup.exe` (`vpn-server-linux-amd64` or `vpn-server-linux-arm64` from this release), or choose the file in the UI. The EXE does not embed the server.

## [v1.4.1] - 2026-08-28

Maintenance / packaging pre-release. Same CONNECT-IP protocol as v1.4.0; existing profiles keep working.

### Added

- **Docker** image and Compose for the server (`server/Dockerfile`, `server/docker-compose.yml`): host network, TUN, NAT via entrypoint.
- Android UI shows **version** (`v1.4.1 (16)`) on phone and TV.
- High-contrast **blue mask** launcher / TV banner icons.

### Fixed

- Server **graceful shutdown** on `SIGTERM`/`SIGINT` (no more `select {}`).
- Android **IPv6 sink** (`::/0` + ULA address) so apps cannot bypass the VPN on dual-stack networks; Go core drops IPv6 datagrams and rewrites wrong IPv4 TUN sources.
- Android TUN uses **`/24`** on-link (server still assigns `/32`) to avoid OEM Wi-Fi source addresses.
- Rebuild TUN when reconnect assigns a different IP (`assigned-ip-changed`).

### Changed

- Android toolchain: **AGP 9.3.2**, **Gradle 9.5.0**, built-in Kotlin (no separate `kotlin-android` plugin). NDK pinned to **27.0.12077973**.
- Windows installer / EXE product version **1.4.1**.

## [v1.4.0] - 2026-08-25

### Added

- **App icon** on Android (launcher, adaptive icon, notification, TV banner) and Windows (tray, window, Start menu, MSI Add/Remove Programs, EXE resources).
- **Ping to server** on the Android and Windows screens: smoothed QUIC RTT to the MASQUE node (not ICMP through the tunnel).
- CI now publishes **linux-amd64** and **linux-arm64** server binaries as workflow artifacts (no certificates inside).
- Repository renamed from `masque-vpn-mvp` to `masque-vpn` (GitHub redirects the old URL).

### Changed

- Server install now **requires 64 MiB UDP socket buffers** (`rmem_max` / `wmem_max`). Documented in `server/README.md`; `masque.service` and `server/sysctl/99-masque-udp.conf` apply `67108864`.

### Tests

- Android stability for this line is complete. After **8 hours of airplane mode** the tunnel came back cleanly.

## [v1.3.1] - 2026-08-23

Pre-release. Automated GitHub Actions checks on this repository pass; there is still no third-party security audit.

### Added

- **Windows desktop client:** LocalSystem service + Fyne GUI (no UAC for daily use) + per-machine MSI. Import `.masque` or toml+`certs/`; the tunnel survives closing the GUI.
- **Android TV:** “Paste config from clipboard” so TVs without working IME paste can import a profile in one click.

### Changed

- README wording: GitHub CI is acknowledged; lack of an independent pentest is still stated plainly.

## [v1.3] - 2026-08-21

Requires a **server upgrade** together with the Android client. A v1.2 server will still accept clients, but reconnect after sleep or a network change can assign a new `/32` and leave the phone TUN silent (`datagram source address not allowed`).

### Added

- **QUIC keepalive** on the client and server (`KeepAlivePeriod` 15s, `MaxIdleTimeout` 3 minutes).
- **Android battery-exemption** prompt before Connect (`REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`).
- **Session reconnect** in the gomobile bridge: the TUN fd stays up; only QUIC/CONNECT-IP is redialed. The VPN service is not stopped on a transport error.
- **Underlying network tracking** on Android (`NetworkCallback`, `setUnderlyingNetworks`, `protect` / `bindSocket` on the QUIC UDP fd) so Wi-Fi → LTE does not leave the socket on a dead path.
- **Sticky tunnel addresses** on the server: the same client certificate CN gets the same `/32` across reconnects.

### Fixed

- Android UI no longer shows “profile ready” / Connect after sleep while the VPN is still running.
- `poc-client` no longer defaults to `InsecureSkipVerify`; a CA (`-ca`) is required (closes CodeQL `go/disabled-certificate-check` on that tool).

### Known limitations

- Long OEM Doze freezes can still halt keepalives; the exemption dialog is not honored on every vendor.
- Airplane mode for many minutes is not a dedicated code path; recovery is the same reconnect + sticky IP.

## [v1.2] - 2026-08-16

### Fixed

- **Android multi-device:** the Android client now uses the address assigned by
  the server (via a two-phase connect) instead of a hardcoded TUN address, so
  multiple Android devices can be connected at the same time. Previously the
  second device connected but received no traffic.
- **Android TV:** "Import from file" no longer freezes the app on TVs without a
  file manager; it detects the missing handler and guides the user to paste-text
  import.

### Added

- **Android TV build** — a dedicated `tv` product flavor (application id
  `com.next1971.masque.tv`) with a leanback launcher, D-pad UI, and paste-text
  profile import. Installs side by side with the phone build.

### Known limitations

- Importing a profile from a file on Android TV is not supported yet (many TVs
  have no file manager). Use paste-text import instead.

### Documentation

- Added [docs/CLIENTS.md](docs/CLIENTS.md): how to issue one config bundle per
  device and add more devices reusing the same CA.

## [v1.1] - 2026-08-16

### Fixed

- **Multi-client routing (server):** a single TUN reader now demultiplexes
  return traffic to the correct client by destination IP, fixing concurrent
  connections from multiple clients.

## [v1.0] - 2026-08-14

### Added

- Initial public MASQUE VPN MVP release.
- Server, Windows client, and Android application sources.
- QUIC + HTTP/3 CONNECT-IP transport with mutual TLS, as described in the project README.

### Notes

- This release is experimental and has not received an independent security audit.
