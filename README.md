# MASQUE VPN

**Self-hosted VPN:** you run the server on your VPS and issue configs only to your own devices. The tunnel rides inside **HTTP/3** (IETF [MASQUE](https://datatracker.ietf.org/doc/html/rfc9484) CONNECT-IP), so it looks like ordinary web traffic rather than a classic VPN handshake. Clients authenticate with **mutual TLS** — the server accepts only certificates you signed.

Clients today: **Android**, **Android TV**, and **Windows** (tray app, no UAC for daily use).

This is not a commercial VPN brand and not an audited enterprise client. It is a working personal or family server with open source. Treat it as **experimental**: GitHub Actions compile and test the tree; that is not a third-party penetration test.

**Why people pick it**

- **Your machine, your keys.** Traffic exits at your VPS. You keep the CA; only issued client certs can connect.
- **HTTP/3 transport.** QUIC + CONNECT-IP is harder to fingerprint than default OpenVPN or WireGuard. It does **not** promise to bypass any particular national firewall.
- **Real clients.** Phone, TV, and Windows installers from [GitHub Releases](../../releases/latest) — not a CLI-only toy.
- **One bundle per device.** Reconnect (sleep, Wi-Fi → LTE, long airplane mode) keeps a stable tunnel address.

Latest line: **v1.4**. Android soak tests for this line are done, including recovery after eight hours of airplane mode. What is next lives in the [roadmap](docs/ROADMAP.md).

## Is this for me?

Use MASQUE VPN if you:
- want to run a VPN server on a VPS you control;
- need Android, Android TV or Windows clients;
- are comfortable creating and managing client credentials;
- want an experimental HTTP/3/QUIC CONNECT-IP implementation.

Do not use it if you:
- need a commercial VPN service or public shared endpoints;
- need anonymous access or a security-audited product;
- need iOS (maybe later- end of 2026), router or browser-extension support today.
  

| What you get                   | Current status            |
| ------------------------------ | ------------------------- |
| Self-hosted server on your VPS | Available                 |
| Android phone/tablet client    | Available                 |
| Android TV client              | Available                 |
| Windows desktop client         | Available                 |
| QUIC tunnel latency in app     | New in v1.4               |
| Client authentication          | mTLS with your private CA |

## Quick start

| Goal | Start here |
|---|---|
| Download the latest build | [Latest release](../../releases/latest) |
| Deploy a Linux server | [Detailed server guide](server/README.md) |
| Connect from Android | [Android guide](android/README.md) |
| Connect from Windows | [Windows guide](windows/README.md) |
| See what's done and what's next | [Roadmap](docs/ROADMAP.md) |
| Report a bug or feature request | [Open an issue](../../issues/new/choose) |
| Report a security issue | [Security policy](SECURITY.md) |

## Repository layout

| Directory | What it is |
|---|---|
| `server/` | Server build instructions, systemd unit, and config generator |
| `windows/` | Windows client (service + GUI + MSI; console remains for debug) |
| `android/` | Android client (Kotlin app + Go core via gomobile) |

The Go source for the server and the shared client core lives under `android/go-src/masque-vpn/` (module `github.com/Next1971/masque-vpn`).

> **Security model.** A single internal Certificate Authority (CA) signs the server certificate and every client certificate. The server only accepts clients whose certificate is signed by that CA, and each client only trusts a server whose certificate is signed by the same CA. **Never commit or publish any `*.key` file** (CA key, server key, client keys).

## 1. Server installation (HOWTO)

The complete, copyable server setup is maintained in [server/README.md](server/README.md). It covers Ubuntu 22.04 prerequisites, a prebuilt Linux binary or a source build, certificates and profiles, systemd/NAT, verification, and the configuration reference.

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Install `masque.msi` from the release (one UAC prompt). That installs the `MasqueVpn` service, `wintun.dll`, and the GUI (tray and Start-menu icon).
2. Open **MASQUE VPN** from the Start menu (no admin). **Import profile**: `profile.masque`.
3. Click **Connect**. The window shows **Ping** (smoothed QUIC RTT to the server). Closing the window does not tear down the tunnel.

---

## 3. Android client

See [`android/README.md`](android/README.md). In short:

1. Install the APK from the release archive (enable "install from unknown sources"), or [build it from source](android/README.md) (**Go 1.25.5+**, **JDK 17**, **compileSdk 36** / **targetSdk 34** / **minSdk 24**, **NDK 27.0.12077973**).
2. Open the app and **import a profile** (`profile.masque` from the generator). On **Android TV**, use **Paste config from clipboard** (or paste-text).
3. Grant the VPN permission and connect. While connected, the screen shows **Ping** (smoothed QUIC RTT to the server).

### Stability testing

Android stability tests for the current line are **complete**. Devices used:

- Honor 200 — MagicOS 10, Android 16
- POCO X4 Pro — Android 13 (TKQ1)
- Haier Android TV

Results:

- **Android TV:** The MASQUE tunnel remained connected for more than 36 hours without interruption on Haier Android TV.
- **Android phones:** Six hours with the screen on completed without tunnel drops. Screen-off / sleep: keepalive plus reconnect; after wake the UI stays on Disconnect when the tunnel is still up.
- **Airplane mode:** after **8 hours** of airplane mode the tunnel came back cleanly (reconnect + sticky `/32`).
- Cell-tower handoff and mobile data → Wi-Fi completed without issues. Wi-Fi → mobile data: 12 switches in a row without losing traffic (underlying network + UDP `protect` + reconnect; server keeps the same `/32` per client certificate).

### Remaining caveats

- Some OEM battery savers still freeze the process; grant “ignore battery optimization” when the app asks. If the process is killed, the user must Connect again.

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
- [Android guide](android/README.md)
- [Windows guide](windows/README.md)
- [Security policy](SECURITY.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
