# MASQUE VPN

> **Status: working.** This project has been operational and tested end-to-end since **July 15, 2026** (server, Windows client, and Android client).

A minimal VPN built on the IETF **MASQUE** framework: it tunnels IP traffic inside HTTP/3 (QUIC) using **CONNECT-IP** (RFC 9484) and authenticates both ends with **mutual TLS (mTLS)**.

> **v1.3.1 is a pre-release.** Builds are checked in GitHub Actions (compile, tests, and the repo’s CI security jobs). That is not a substitute for a third-party penetration test; treat the project as experimental.

## Quick start

| Goal | Start here |
|---|---|
| Download the latest build | [Latest release](../../releases/latest) |
| Deploy a Linux server | [Detailed server guide](server/README.md) |
| Connect from Android | [Android guide](android/README-Android.md) |
| Connect from Windows | [Windows guide](windows/README.md) |
| See what's done and what's next | [Roadmap](docs/ROADMAP.md) |
| Report a bug or feature request | [Open an issue](../../issues/new/choose) |
| Report a security issue | [Security policy](SECURITY.md) |

The repository contains three parts:

| Directory | What it is |
|---|---|
| `server/` | Server build instructions, systemd unit, and config generator |
| `windows/` | Windows client (service + GUI + MSI; console remains for debug) |
| `android/` | Android client (Kotlin app + Go core via gomobile) |

The Go source for the server and the shared client core lives under `android/go-src/masque-vpn-mvp/` (one Go module, `github.com/Next1971/masque-vpn-mvp`) and is used to build both the server and the Android core.

> **Security model.** A single internal Certificate Authority (CA) signs the server certificate and every client certificate. The server only accepts clients whose certificate is signed by that CA, and each client only trusts a server whose certificate is signed by the same CA. **Never commit or publish any `*.key` file** (CA key, server key, client keys).

> [!IMPORTANT]
> ## MVP status: core hypothesis validated
>
> The primary MVP goal has been achieved: a MASQUE tunnel based on
> QUIC + HTTP/3 CONNECT-IP with mutual TLS works end-to-end in real use.
>
> The project is still experimental and is not yet a production-ready VPN
> client. The v1.x line focuses on reliability, network transitions, and
> client experience rather than changing the core tunnel design.

## Roadmap

### v1.x — Reliability and client experience

The core MASQUE tunnel is working. The next releases focus on making the Android and Windows clients resilient during normal mobile-device usage:

- Better recovery after Wi-Fi ↔ mobile-network switching (**v1.3**)
- Improved background behaviour while the screen is off (**v1.3**: QUIC keepalive, battery-exemption prompt, reconnect without tearing the TUN)
- Automatic reconnection after temporary loss of connectivity, including recovery after airplane mode is turned off
- MTU experiments and tuning for different network conditions
- A full Windows desktop client (**v1.3.1 pre-release**: LocalSystem service + GUI without UAC + MSI)

### v2.0 — Platform and protocol upgrade

Version 2.0 is planned as a major upgrade:

- IPv6 support
- Major upgrades of the Android Gradle Plugin and Gradle wrapper

> [!NOTE]
> AGP 9.x and Gradle 9.x are a coordinated migration. They require Kotlin 2.x and target SDK changes, so this work will be planned and tested deliberately, not accepted through automated dependency-update pull requests. Minor and patch updates, including security fixes, remain welcome.

---

## 1. Server installation (HOWTO)

The complete, copyable server setup is maintained in [server/README.md](server/README.md). It covers Ubuntu 22.04 prerequisites, building the server, certificates and profiles, systemd/NAT, verification, and the configuration reference.

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Install `masque.msi` from the release (one UAC prompt). That installs the `MasqueVpn` service, `wintun.dll`, and the GUI.
2. Open **MASQUE VPN** from the Start menu (no admin). **Import profile**: `profile.masque`, or `profile.client.toml` together with its `certs/` folder.
3. Click **Connect**. Closing the window does not tear down the tunnel.

Console debug (admin): `vpn-client.exe -profile profile.client.toml -full-route`.

**"Disable certificate verification" toggle.** Some setups fail the TLS handshake on server certificate validation (self-signed CA, hostname mismatch). The client offers an opt-in switch to skip that check:

- Web UI: tick **"Disable certificate verification"** before connecting.
- Console: add the `-insecure` flag.
- Profile: set `insecure = true` under `[tls]`.

It is **off by default** (secure). Only enable it while troubleshooting — it disables authentication of the server.

---

## 3. Android client

See [`android/README-Android.md`](android/README-Android.md). In short:

1. Install the APK from the release archive (enable "install from unknown sources"), or build it yourself (Android SDK 34, NDK r27c, JDK 17):

   ```bash
   cd android
   # Build the Go core into an .aar:
   #   (Windows)  scripts\build-aar.bat
   #   (Linux)    see android/README-Android.md
   ./gradlew :app:assembleRelease
   ```

2. Open the app and **import a profile** (`profile.masque` from the generator). The APK also ships a non-production `sample-profile.masque` in its assets so you can see the expected format.
3. Grant the VPN permission and connect.

### Stability testing

Current testing was performed over Wi-Fi on:

- Honor 200 — MagicOS 10, Android 16
- POCO X4 Pro — Android 13 (TKQ1)
- Haier Android TV

- **Android TV:** The MASQUE tunnel remained connected for more than 36 hours without interruption on Haier Android TV.
- **Android phones:** Six hours of continuous testing with the screen kept on completed without VPN tunnel disconnects or noticeable connectivity drops. **v1.3** additionally covers screen-off / sleep: keepalive plus reconnect; after wake the UI stays on Disconnect when the tunnel is still up.

### Network transitions

- Switching between mobile network cell towers completed without issues in current testing.
- Switching from mobile data to Wi-Fi completed without issues in current testing.
- **v1.3:** Wi-Fi → mobile data is recovered by updating the VPN underlying network, protecting the QUIC UDP socket, and reconnecting the MASQUE session if QUIC dies. The server keeps the same tunnel `/32` for a given client certificate so the Android TUN does not go silent after reconnect. Lab check: 12 Wi-Fi ↔ LTE switches in a row without losing traffic.

### Remaining caveats

- Some OEM battery savers still freeze the process; grant “ignore battery optimization” when the app asks. If the process is killed, the user must Connect again.
- Airplane mode for a long stretch is still best treated as a reconnect-after-wake case, not a guaranteed instant restore.

---

## 4. Security notes

- **Never commit secrets.** `*.key`, keystores, `keystore.properties`, and real client profiles are all git-ignored. Distribute client bundles out-of-band.
- **Back up the CA.** Losing `ca.key` means you can no longer issue new client certificates without re-provisioning every client.
- `insecure = false` is the safe default on every client. Enable it only to work around a certificate problem you understand.
- Read [SECURITY.md](SECURITY.md) before deploying the project for real users.

## 5. Protocol / stack

- **QUIC + HTTP/3** transport (`quic-go`).
- **CONNECT-IP** (RFC 9484) via `connect-ip-go` for IP-level tunneling.
- **Wintun** TUN driver on Windows; Android `VpnService` file descriptor on Android; native TUN on Linux.
- **mTLS** with an internal EC (P-256) CA for mutual authentication.

## Project documentation

- [Roadmap](docs/ROADMAP.md)
- [Issuing client configs (one bundle per device)](docs/CLIENTS.md)
- [Server installation guide](server/README.md)
- [Android guide](android/README-Android.md)
- [Windows guide](windows/README.md)
- [Security policy](SECURITY.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)