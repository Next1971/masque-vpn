# MASQUE VPN

**Self-hosted VPN:** you run the server on your VPS and issue configs only to your own devices. The tunnel rides inside **HTTP/3** (IETF [MASQUE](https://datatracker.ietf.org/doc/html/rfc9484) CONNECT-IP), so it looks like ordinary web traffic rather than a classic VPN handshake. Clients authenticate with **mutual TLS** — the server accepts only certificates you signed.

Clients today: **Android**, **Android TV**, and **Windows** (tray app, no UAC for daily use). **iOS** is on TestFlight ([public link](https://testflight.apple.com/join/x52N41V1), **v1.7**); no IPA is attached to GitHub Releases.

This is not a commercial VPN brand and not an audited enterprise client. It is a working personal or family server with open source. Treat it as **experimental**: GitHub Actions compile and test the tree; that is not a third-party penetration test.

**Why people pick it**

- **Your machine, your keys.** Traffic exits at your VPS. You keep the CA; only issued client certs can connect.
- **HTTP/3 transport.** QUIC + CONNECT-IP is harder to fingerprint than default OpenVPN or WireGuard. It does **not** promise to bypass any particular national firewall.
- **Real clients.** Phone, TV, and Windows installers from [GitHub Releases](../../releases/latest) — not a CLI-only toy.
- **One bundle per device.** Reconnect (sleep, Wi-Fi → LTE, long airplane mode) keeps a stable tunnel address.

## Release status

- **Latest:** [v1.5.3](../../releases/tag/v1.5.3) — daily driver. IPv6 in the tunnel, optional kill switch (default off), Android TV Connect fix (MSI, APKs; same Linux server as v1.5.0/1.5.1).
- **v1.7 (iOS TestFlight):** [v1.7](../../releases/tag/v1.7) — public TestFlight [join link](https://testflight.apple.com/join/x52N41V1), Apple build **20**. Expires **4 December 2026** (80 days); a new 90-day build follows. No APK/MSI/server on this tag. **Latest stays v1.5.3.**
- **v1.5.6 (optional):** [v1.5.6](../../releases/tag/v1.5.6) — QUIC to the server over IPv6, Windows Disconnect fix, dual-stack server bind, `install.sh`. Previous 1.5.x clients and servers stay compatible over IPv4. **If your setup already works, you do not need to install 1.5.6.**
- **v1.5.4 (pre-release):** `connect-ip-go` v0.3.0 + optional `alt_port` dual-port. New Linux server binary.
- **v1.5.2 (pre-release):** optional **kill switch** on Android and Windows (default off). Same protocol; no server binary change.
- **v1.5.1 (pre-release):** CN denylist / Windows revoke, TUN MTU **1369**, UDP 443 VPS redirect.

  What is next lives in the [roadmap](docs/ROADMAP.md) (**v1.8** phone QR import).

## Is this for me?

Use MASQUE VPN if you:
- want to run a VPN server on a VPS you control;
- need Android, Android TV or Windows clients;
- are comfortable creating and managing client credentials;
- want an experimental HTTP/3/QUIC CONNECT-IP implementation.

Do not use it if you:
- need a commercial VPN service or public shared endpoints;
- need anonymous access or a security-audited product;
- need an App Store iOS client (TestFlight is [here](https://testflight.apple.com/join/x52N41V1); no IPA on GitHub), router or browser-extension support;
  
## Choose your installation path

| Path | Recommended for | Stability | Documentation |
|---|---|---|---|
| Manual SSH install | Users who want to inspect every server-side command | Recommended | [Server guide](server/README.md) |
| Docker Compose | Users familiar with Docker on a dedicated Linux VPS | Experimental | [Docker guide](server/README.md#docker) |
| Windows VPS Setup Helper | Testers using a disposable VPS | Test pre-release only Ubuntu 22.04, 24.04, or 26.04, or Debian 12 | [masque-setup.exe](windows/README.md#install-the-server-from-windows-masque-setupexe) |

## Quick start

| Goal | Start here |
|---|---|
| Download Latest (v1.5.3) | [GitHub Release](../../releases/latest) |
| Install iOS (TestFlight v1.7) | [Public TestFlight link](https://testflight.apple.com/join/x52N41V1) |
| Download v1.5.6 (optional IPv6 QUIC) | [v1.5.6](../../releases/tag/v1.5.6) |
| Download v1.5.4 pre-release | [v1.5.4](../../releases/tag/v1.5.4) |
| Deploy a Linux server | [Detailed server guide](server/README.md) |
| Connect from Android | [Android guide](android/README.md) |
| Connect from iOS (source / TestFlight) | [iOS guide](ios/README.md) |
| Connect from Windows | [Windows guide](windows/README.md) |
| Install the server from Windows | [masque-setup.exe](windows/README.md#install-the-server-from-windows-masque-setupexe) |
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

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Install `masque.msi` from the release (one UAC prompt). That installs the `MasqueVpn` service, `wintun.dll`, and the GUI (tray and Start-menu icon).
2. Open **MASQUE VPN** from the Start menu (no admin). **Import profile**: `profile.masque`.
3. Click **Connect**. The window shows **Ping** (smoothed QUIC RTT to the server). Closing the window does not tear down the tunnel.

---

## 3. Android client

See [`android/README.md`](android/README.md). In short:

1. Install the APK from the release archive (enable "install from unknown sources"), or [build it from source](android/README.md) (**Go 1.25.5+**, **JDK 17**, **compileSdk 36** / **targetSdk 34** / **minSdk 24**, **NDK 29.0.13599879**).
2. Open the app and **import a profile** (`profile.masque` from the generator). On **Android TV**, use **Paste config from clipboard** (or paste-text).
3. Grant the VPN permission and connect. While connected, the screen shows **Ping** (smoothed QUIC RTT to the server).


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
- [Android guide](android/README.md)
- [Windows guide](windows/README.md)
- [Security policy](SECURITY.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
