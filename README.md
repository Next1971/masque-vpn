# MASQUE VPN

> **Status: working.** This project has been operational and tested end-to-end since **July 15, 2026** (server, Windows client, and Android client).

A minimal VPN built on the IETF **MASQUE** framework: it tunnels IP traffic inside HTTP/3 (QUIC) using **CONNECT-IP** (RFC 9484) and authenticates both ends with **mutual TLS (mTLS)**.

> **v1.0 is released.** This is experimental software and has not received an independent security audit.

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
| `windows/` | Windows client (console + local web UI) |
| `android/` | Android client (Kotlin app + Go core via gomobile) |

The Go source for the server and the shared client core lives under `android/go-src/masque-vpn-mvp/` (one Go module, `github.com/Next1971/masque-vpn-mvp`) and is used to build both the server and the Android core.

> **Security model.** A single internal Certificate Authority (CA) signs the server certificate and every client certificate. The server only accepts clients whose certificate is signed by that CA, and each client only trusts a server whose certificate is signed by the same CA. **Never commit or publish any `*.key` file** (CA key, server key, client keys).

---

## 1. Server installation (HOWTO)

The complete, copyable server setup is maintained in [server/README.md](server/README.md). It covers Ubuntu 22.04 prerequisites, building the server, certificates and profiles, systemd/NAT, verification, and the configuration reference.

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Get `vpn-client.exe` and `wintun.dll` from the release archive (or build with `windows/scripts/build.ps1`).
2. Put the `windows/` bundle from the generator next to the EXE: this gives you `profile.client.toml` and a `certs/` folder.
3. Run `vpn-client.exe` (no arguments) and open <http://localhost:8080> — select the profile, then click **CONNECT**.
   - Console mode alternative: `vpn-client.exe -profile profile.client.toml -full-route`.

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
- [Server installation guide](server/README.md)
- [Android guide](android/README-Android.md)
- [Windows guide](windows/README.md)
- [Security policy](SECURITY.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
