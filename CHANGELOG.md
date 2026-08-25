# Changelog

All notable changes to MASQUE VPN are documented here.

## [Unreleased]

## [v1.4.0] - 2026-08-25

### Added

- **App icon** on Android (launcher, adaptive icon, notification, TV banner) and Windows (tray, window, Start menu, MSI Add/Remove Programs, EXE resources).
- **Ping to server** on the Android and Windows screens: smoothed QUIC RTT to the MASQUE node (not ICMP through the tunnel).
- CI now publishes **linux-amd64** and **linux-arm64** server binaries as workflow artifacts (no certificates inside).
- Repository renamed from `masque-vpn-mvp` to `masque-vpn` (GitHub redirects the old URL).

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
