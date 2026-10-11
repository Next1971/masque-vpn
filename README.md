# MASQUE VPN

**Self-hosted VPN:** you run the server on your VPS and issue configs only to your own devices. The default transport is IETF [MASQUE](https://datatracker.ietf.org/doc/html/rfc9484) CONNECT-IP over **HTTP/3 (QUIC)**. The experimental **v2.0.0** release also supports CONNECT-IP over **HTTP/2 (TCP)**, used automatically when the UDP probe receives no reply. Clients authenticate with **mutual TLS** — the server accepts only certificates you signed.

Clients today: **Android**, **Android TV**, and **Windows** (tray app, no UAC for daily use). **iOS** is on TestFlight ([public link](https://testflight.apple.com/join/x52N41V1), **v1.7**); no IPA is attached to GitHub Releases.

This is not a commercial VPN brand and not an audited enterprise client. It is a working personal or family server with open source. Treat it as **experimental**: GitHub Actions compile and test the tree; that is not a third-party penetration test.

**Why people pick it**

- **Your machine, your keys.** Traffic exits at your VPS. You keep the CA; only issued client certs can connect.
- **HTTP/3 transport.** QUIC + CONNECT-IP is harder to fingerprint than default OpenVPN or WireGuard. It does **not** promise to bypass any particular national firewall.
- **Real clients.** Phone, TV, and Windows installers from [GitHub Releases](../../releases/latest) — not a CLI-only toy.
- **One bundle per device.** Reconnect (sleep, Wi-Fi → LTE, long airplane mode) keeps a stable tunnel address.

## Release status

- **Latest:** [v1.7.1](../../releases/tag/v1.7.1) — Android APK and Windows MSI on `connect-ip-go` v0.4.0. They connect to the 1.5.x server already running. No new server binary in this drop.
- **v2.0.0 (experiment, not Latest):** [v2.0.0](../../releases/tag/v2.0.0) — phone APK, TV APK, Windows MSI, Linux server, `install.sh`, and Docker sources (`masque-docker-2.0.0.zip`). Client tunnel MTU **1340**, with no MTU field or TCP/UDP switch. UDP is the normal path; TCP HTTP/2 CONNECT-IP opens only when the UDP probe receives no reply, on the same host and ports. Profiles and certificates do not change. `masque-setup.exe` is not attached (that wizard still opens UDP only). iOS stays TestFlight **v1.7**.
- **v1.7 (iOS TestFlight):** [v1.7](../../releases/tag/v1.7) — public TestFlight [join link](https://testflight.apple.com/join/x52N41V1), Apple build **20**. Expires **4 December 2026**. No APK/MSI/server on this tag.
- **v1.5.6 (optional earlier release):** [v1.5.6](../../releases/tag/v1.5.6) — QUIC to the server over IPv6, Windows Disconnect fix, dual-stack server bind, and `install.sh`. Previous 1.5.x clients and servers stay compatible over IPv4.

**If your current VPN works, you do not need to install v2.0.0.** Latest remains v1.7.1.

**TCP fallback requires a v2.0.0 server.** A v2.0.0 client can still use an older server over UDP, but cannot use TCP fallback with it. Older UDP clients, including v1.7.1, can connect to a v2.0.0 server over UDP.

See the [changelog](CHANGELOG.md) for earlier releases and the [roadmap](docs/ROADMAP.md) for future work.

## Is this for me?

Use MASQUE VPN if you:
- want to run a VPN server on a VPS you control;
- need Android, Android TV or Windows clients;
- are comfortable creating and managing client credentials;
- want an experimental HTTP/3/QUIC CONNECT-IP implementation, with optional testing of HTTP/2/TCP fallback in v2.0.0.

Do not use it if you:
- need a commercial VPN service or public shared endpoints;
- need anonymous access or a security-audited product;
- need an App Store iOS client (TestFlight is [here](https://testflight.apple.com/join/x52N41V1); no IPA on GitHub), router or browser-extension support.

## Choose your installation path

| Path | Recommended for | Stability / limitations | Documentation |
|---|---|---|---|
| Manual SSH install | Users who want to inspect every server-side command | Recommended installation path; use the v2.0.0 `install.sh` when testing TCP fallback | [Server guide](server/README.md) |
| Docker Compose | Users familiar with Docker on a dedicated Linux VPS | Experimental; v2.0.0 Docker sources are available in that release | [Docker guide](server/README.md#docker) |
| Windows VPS Setup Helper | Testers using a disposable VPS | Experimental; Ubuntu 22.04, 24.04, or 26.04, or Debian 12. UDP-only setup; not suitable for deploying v2.0.0 TCP fallback | [masque-setup.exe](windows/README.md#install-the-server-from-windows-masque-setupexe) |

## Quick start

| Goal | Start here |
|---|---|
| Download Latest (v1.7.1) | [GitHub Release](../../releases/latest) |
| Test v2.0.0 (new server required for TCP fallback) | [v2.0.0 files and deployment notes](../../releases/tag/v2.0.0) |
| Install iOS (TestFlight v1.7) | [Public TestFlight link](https://testflight.apple.com/join/x52N41V1) |
| Download v1.5.6 (optional earlier IPv6 QUIC release) | [v1.5.6](../../releases/tag/v1.5.6) |
| Deploy a Linux server | [Detailed server guide](server/README.md) |
| Connect from Android | [Android guide](android/README.md) |
| Connect from iOS (source / TestFlight) | [iOS guide](ios/README.md) |
| Connect from Windows | [Windows guide](windows/README.md) |
| Install a UDP-only server from Windows (experimental) | [masque-setup.exe](windows/README.md#install-the-server-from-windows-masque-setupexe) |
| See what's done and what's next | [Roadmap](docs/ROADMAP.md) |
| Report a bug or feature request | [Open an issue](../../issues/new/choose) |
| Report a security issue | [Security policy](SECURITY.md) |

## Repository layout

| Directory | What it is |
|---|---|
| `server/` | Server build instructions, systemd unit, Docker, and config generator |
| `windows/` | Windows client (service + GUI + MSI; console remains for debug) |
| `android/` | Android client (Kotlin app + Go core via gomobile) |
| `ios/` | iOS client (Swift app + Packet Tunnel + Go core via gomobile) |

The Go source for the server and the shared client core lives under `android/go-src/masque-vpn/` (module `github.com/Next1971/masque-vpn`).

> **Security model.** A single internal Certificate Authority (CA) signs the server certificate and every client certificate. The server only accepts clients whose certificate is signed by that CA, and each client only trusts a server whose certificate is signed by the same CA. **Never commit or publish any `*.key` file** (CA key, server key, client keys).

## 1. Server installation (HOWTO)

The complete, copyable server setup is maintained in [server/README.md](server/README.md). It covers Ubuntu 22.04 prerequisites, a prebuilt Linux binary or a source build, **optional Docker Compose**, certificates and profiles, **64 MiB UDP socket buffers** (required for QUIC), systemd/NAT, verification, and the configuration reference.

For the v2.0.0 experiment, use its [release files and deployment notes](../../releases/tag/v2.0.0). Install the new server with `install.sh` or build the Docker sources from that tag. The Windows VPS Setup Helper is not the installation path for TCP fallback.

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Install the MSI from the selected release (one UAC prompt). That installs the `MasqueVpn` service, `wintun.dll`, and the GUI (tray and Start-menu icon).
2. Open **MASQUE VPN** from the Start menu (no admin). **Import profile**: `profile.masque`.
3. Click **Connect**. On QUIC connections, **Ping** is the smoothed QUIC RTT to the server, not a transport-independent latency measurement. Closing the window does not tear down the tunnel.

---

## 3. Android client

See [`android/README.md`](android/README.md) for full details. In short:

1. Install the APK from the release archive (enable "install from unknown sources"), or [build it from source](android/README.md) (**Go 1.25.5+**, **JDK 17**, **compileSdk 36** / **targetSdk 34** / **minSdk 24**, **NDK 29.0.13599879**).
2. Open the app and **import a profile** (`profile.masque` from the generator). On **Android TV**, use **Paste config from clipboard** (or paste-text).
3. Grant the VPN permission and connect. On QUIC connections, **Ping** is the smoothed QUIC RTT to the server, not a transport-independent latency measurement.

## 4. Security notes

- **Never commit secrets.** `*.key`, keystores, `keystore.properties`, and real client profiles are all git-ignored. Distribute client bundles out-of-band.
- **Back up the CA.** Losing `ca.key` means you can no longer issue new client certificates without re-provisioning every client.
- `insecure = false` is the safe default on every client. Enable it only to work around a certificate problem you understand.
- Read [SECURITY.md](SECURITY.md) before deploying the project for real users.

## 5. Protocol / stack

- **QUIC + HTTP/3** default transport (`quic-go`).
- **CONNECT-IP** (RFC 9484) via `connect-ip-go` for IP-level tunneling on the QUIC path.
- **HTTP/2 + TCP** CONNECT-IP capsule transport in experimental v2.0.0, used when the UDP probe receives no reply. Requires the v2.0.0 server; iOS TestFlight v1.7 remains UDP-only.
- **Wintun** TUN driver on Windows; Android `VpnService` file descriptor on Android; native TUN on Linux.
- **mTLS** with an internal EC (P-256) CA for mutual authentication.

## Project documentation

- [Roadmap](docs/ROADMAP.md)
- [Issuing client configs (one bundle per device)](docs/CLIENTS.md)
- [Server installation guide](server/README.md)
- [Android guide](android/README.md)
- [iOS guide](ios/README.md)
- [Windows guide](windows/README.md)
- [Security policy](SECURITY.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
