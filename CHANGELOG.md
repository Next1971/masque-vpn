# Changelog

All notable changes to MASQUE VPN are documented here.

## [Unreleased]

## [v1.7] - 2026-09-15

**GitHub pre-release; Latest stays [v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3).** iOS TestFlight only. No APK, MSI, or Linux server on this tag. Optional IPv6 QUIC remains [v1.5.6](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.6).

### Added

- Public TestFlight: [https://testflight.apple.com/join/x52N41V1](https://testflight.apple.com/join/x52N41V1). Marketing **1.7.0**, Apple build **20** (external testing approved). Built from [Actions run 20](https://github.com/Next1971/masque-vpn/actions/runs/33980112341) on `e708ea7`.

### Notes

- Android, Windows, and the Linux server stay on Latest **v1.5.3** (or optional **v1.5.6**). This TestFlight binary does not include later `main` iOS work (`alt_port`, QUIC over IPv6). Use an IPv4 profile and a single UDP port.

## [v1.5.6] - 2026-09-14

**GitHub release; Latest stays [v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3).** QUIC to the server over IPv6 (AAAA or an IPv6 literal). Linux full-route adds a `/128` host bypass so the UDP socket does not loop into TUN. Same CONNECT-IP protocol as v1.5.4. Android `1.5.6` (`versionCode` 22); Windows product **1.5.6**. **If a previous version already works, you do not need to install 1.5.6.**

### Fixed

- Windows: **Disconnect** closes the CONNECT-IP/QUIC session immediately. Previously the service only cancelled a context while `ReadPacket` stayed blocked, so the GUI stayed on Connected until a packet arrived or you stopped the `MasqueVpn` service.

### Added

- Client resolves A and AAAA; IPv6 is preferred. The QUIC UDP socket matches the peer family (`udp4` / `udp6`).
- `gen-config.sh --dial` writes a bracketed IPv6 address into client profiles. Server bind defaults to `[::]:port`.
- `install.sh` accepts optional `MASQUE_IP` (extra SAN) and `MASQUE_DIAL` (client QUIC address).

### Notes

- All earlier 1.5.x clients reach a 1.5.6 server over IPv4; a 1.5.6 client reaches an older IPv4-only server when the profile is IPv4 (or a name without a real AAAA). Do not publish AAAA for hostnames still used by pre-1.5.6 clients.
- `masque-setup.exe` still picks one UDP port and an IPv4/DNS public host. Dual-port and IPv6 literals stay in `gen-config.sh` / `install.sh`.

## [v1.5.5] - 2026-09-11

**Not published as a GitHub tag.** Server install packaging (`install.sh`, SHA256SUMS, Sigstore). Same CONNECT-IP protocol as v1.5.4. Those files ship in **v1.5.6**. v1.5.3 remains **Latest**.

### Added

- Public `server/install.sh`: as root, with `MASQUE_HOST` and `MASQUE_PORT`, downloads the matching Linux server binary and `gen-config.sh` from this GitHub Release, verifies `SHA256SUMS`, and installs `/opt/masque` (same layout as `masque-setup.exe`). If those files are already on disk (Windows helper upload), it skips the download.
- `SHA256SUMS` on the release. Sigstore / GitHub artifact attestations on the CI-built Linux server binaries and install scripts (`gh attestation verify <file> --repo Next1971/masque-vpn`).
- `.github/workflows/release.yml` on `v*` tags: Linux amd64/arm64, checksums, attestations, upload.

### Changed

- `masque-setup.exe` accepts Ubuntu **26.04** LTS (in addition to 22.04 / 24.04 and Debian 12). Clients stay **v1.5.4** (no APK/MSI rebuild).

### Notes

- Next on main, no extra pre-release: AGP 9.4 / Gradle 9.6 (Dependabot #75). **v1.5.6** is QUIC over IPv6.

## [v1.5.4] - 2026-09-10

**GitHub pre-release.** Library bump (`connect-ip-go` v0.3.0) plus dual-port QUIC dial. Same CONNECT-IP protocol; existing profiles keep working. **New Linux server binary** (first server drop since v1.5.1). v1.5.3 remains **Latest** until this soaks.

### Added

- Optional `[server].alt_port` in the profile: Android, Windows, and Linux clients race that UDP port and `[server].server`. First successful QUIC handshake wins; CONNECT-IP runs only on the winner. Omit `alt_port` for a single port (same as v1.5.3). iOS stores the field but still dials the profile port only.

### Changed

- `connect-ip-go` **v0.2.0 → v0.3.0** (server and clients). `quic-go` stays **v0.62.0** (what v0.3.0 pins).
- Android `1.5.4` (`versionCode` 21); Windows product **1.5.4**.

### Notes

- Dual-port is not guessed: write `alt_port = 2053` (or any other port) in the profile, and open that port on the VPS (listen or DNAT). `gen-config.sh --alt-port` writes the field.
- DoH/DoT and extra numbered-profile UI are not in this tag.

## [v1.5.3] - 2026-09-09

**GitHub Latest.** Client line after v1.5.2: Android TV Connect no longer crashes. Ships optional **kill switch** (default off) and optional IPv6 in the tunnel. Same CONNECT-IP protocol; existing profiles keep working. No new Linux server binary is required.

### Fixed

- Android TV: Connect crashed when building the VPN notification — TV has only `LEANBACK_LAUNCHER`, so `getLaunchIntentForPackage()` was null. Use the leanback launch intent.
- Android TV: skip the battery-exemption Settings screen (often missing on leanback).
- Android: if `VpnService` rejects IPv6 addresses or `::/0`, continue with IPv4 only.

### Changed

- Android `1.5.3` (`versionCode` 20); Windows product **1.5.3**.

### Notes

- Next client/server slice is **v1.5.4**: library bump, then Android dual-port. Numbered issue files already ship; DoH/DoT is out of scope (self-hosted VPS). iOS TestFlight stays **v1.7** (not later than **12 October 2026**).

## [v1.5.2] - 2026-09-09

**GitHub pre-release.** Optional **kill switch** on Android and Windows. Same CONNECT-IP protocol; existing profiles keep working. Default is **off**. Superseded as Latest by **v1.5.3**.

### Added

- Android phone/TV: checkbox **Kill switch**. While the VPN session is up, `VpnService.setBlocking(true)` and a held TUN so apps cannot fall back to the underlay if the QUIC session dies. Disconnect still tears the TUN down.
- Windows GUI/service: the same toggle in `settings.json` (`kill_switch`). Routes go up before the first dial; they stay until **Disconnect**, so a dead tunnel blackholes traffic instead of restoring the physical default gateway.

### Notes

- Kill switch does not survive a killed VPN process (OEM battery saver, `taskkill`). On Android, also enable system **Always-on VPN** and **Block connections without VPN**.
- Changing the checkbox while already connected applies on the next Connect (Android blocking flag is set when TUN is established).

## [v1.5.1] - 2026-09-06

**GitHub pre-release.** Maintenance line after v1.5.0. Same CONNECT-IP protocol; existing profiles keep working. v1.5.0 remains **Latest**.

### Added

- Server CN denylist: `/opt/masque/blocked_cns` (one Common Name per line) plus optional `tls.blocked_cns` in `config.server.toml`. Reload requires a process restart.
- `masque-setup.exe`: **Revoke certificate** writes the CN to that file and restarts `masque.service`.
- Documented VPS UDP redirect when outbound **UDP 443** is dropped: listen on 443, DNAT/REDIRECT an alternate port (typically **2053**) to 443, point the client profile at `:2053`.

### Changed

- Client tunnel MTU default **1369** (Android `versionCode` 18, Windows product **1.5.1**, new `gen-config.sh` profiles). Chosen from path MTU tests: MTS mobile data DF-safe at 1370, loss at 1380; one-byte margin. See `docs/benchmarks/mtu.md`.
- Docker image tag `masque-vpn-server:1.5.1`.

### Notes

- First successful **iOS** on-device result. No iOS binary is attached to this GitHub release. TestFlight access is planned for **v1.6** (not later than **12 October 2026**).
- Android simultaneous dual-port (UDP 443 and the alternate port at once) is planned for **v1.6**. Until then use the VPS redirect and a single port in the profile.

## [v1.5.0] - 2026-08-30

**Was GitHub Latest** until v1.5.3. Optional **IPv6 inside the tunnel**. Existing IPv4-only server configs (no `tun_addr_v6` / `pool_cidr_v6`) behave as before. Client profiles do not change. QUIC to the server stays on IPv4; do not add an AAAA for the VPN hostname in this release.

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
