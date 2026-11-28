# MASQUE VPN

[![Donate Bitcoin](https://img.shields.io/badge/Donate-Bitcoin-F7931A?logo=bitcoin&logoColor=white)](DONATE.md#bitcoin-btc)
[![Donate Monero](https://img.shields.io/badge/Donate-Monero-FF6600?logo=monero&logoColor=white)](DONATE.md#monero-xmr)

Self-hosted MASQUE CONNECT-IP VPN: your VPS, your certificate authority, one profile per device. HTTP/3 over QUIC (UDP) is the normal transport; HTTP/2 over TCP is available on the same port number. VPN access requires a valid client certificate.

Clients: Android, Android TV, Windows, and iOS 2.2.0 via TestFlight by 7 December 2026 (the published TestFlight build stays 2.1.1 until then). No IPA is attached to GitHub Releases.

Experimental software, not a commercial VPN service or an independently security-audited product. No guarantee of bypassing any particular firewall.

## Current release: v2.2.0 (clients) · server v2.1.0

[Download v2.2.0](../../releases/tag/v2.2.0) · [Latest release](../../releases/latest) · [Changelog](CHANGELOG.md)

**Clients in Latest:** Android phone, Android TV, and Windows **v2.2.0**, plus the `masque-setup.exe` helper. **Server stays [v2.1.0](../../releases/tag/v2.1.0)** — do not replace the VPS binary for this release. iOS: **2.2.0 arrives on [TestFlight](https://testflight.apple.com/join/x52N41V1) by 7 December 2026**; the published TestFlight build stays 2.1.1 until then. No IPA in this drop.

### What changed in the clients

- **Multiple profiles on every platform:** store several imported profiles and pick the active one in the app. Each profile lives in its own directory; the id combines the server name with the client certificate hash, so re-importing a bundle updates that entry instead of creating a duplicate. Pre-2.2 single-profile installs migrate automatically.
- Android phone: profile spinner with Delete. Android TV: a compact picker row one D-pad press above Connect; Connect stays the first control.
- Windows: profile dropdown with Delete. The service gained `list_profiles`, `set_active`, and `delete_profile` over the named pipe.
- Switching profiles while connected disconnects the current session first, after a confirmation. Transport, MTU, kill switch, autoconnect, and traffic counters stay global per device.
- The attached `masque-setup.exe` again defaults its embedded install script to the **v2.1.0** server, and its wizard fits smaller 14-inch displays.
- Dependency updates since v2.1.3: connect-ip-go 0.7.0 with quic-go v0.65.0, golang.org/x/net 0.63.0, golang.org/x/crypto 0.59.0 (Windows), golang.org/x/sys 0.50.0 (Android), Android Gradle plugin 9.5.2, Gradle wrapper 9.9.1.

### Server (unchanged)

- HTTP/3 CONNECT-IP over UDP and HTTP/2 CONNECT-IP over TCP on the same port number.
- TCP: a valid client certificate plus an HTTP/2 CONNECT-IP request opens the VPN. Browsers and connections without a valid client certificate receive an HTML page, not VPN access.
- The server uses a private-CA certificate. A browser shows a certificate warning before the page; this is not a publicly trusted HTTPS website. UDP does not serve a website.

Android, Android TV, and Windows still offer Automatic / UDP only / TCP only, configurable MTU (default 1340), kill switch (off by default), daily/monthly traffic statistics, and ping during TCP sessions. QUIC ping is the smoothed QUIC RTT; do not interpret TCP ping as QUIC RTT. The published iOS 2.1.1 TestFlight build does UDP and TCP; 2.2.0 replaces it by 7 December 2026.

### Compatibility and upgrades

Existing profiles and certificates remain unchanged. Install the v2.2.0 APK or MSI over the previous client. Keep the v2.1.0 server. A v1.7.1 client can still connect to that server over UDP. TCP requires a v2.0.0 or v2.1.0 server. iOS 2.1.1 uses the same server over UDP or TCP.

For earlier release details, use the [changelog](CHANGELOG.md) and the corresponding release notes.

## Choose your installation path

| Path | Status and requirements | Guide |
|---|---|---|
| Manual SSH installation | Recommended; Linux VPS you control; review commands before running them | [Server guide](server/README.md) |
| Docker Compose | Experimental; host networking, NET_ADMIN, /dev/net/tun; sources in masque-docker-2.1.0.zip, image tag masque-vpn-server:2.1.0 | [Docker guide](server/README.md#docker) |
| Windows VPS Setup Helper | Experimental; masque-setup.exe with the Linux server binary beside it; v2.1.0 installs UDP and TCP | [Setup Helper guide](windows/README.md#install-the-server-from-windows-masque-setupexe) |

v2.2.0 attaches the phone APK, TV APK, Windows MSI, and `masque-setup.exe`. Server binaries, `install.sh`, `gen-config.sh`, and Docker sources remain on [v2.1.0](../../releases/tag/v2.1.0). The Docker image is not supplied as a tarball.

install.sh is for a new server installation: it verifies SHA256SUMS and refuses to replace an existing CA, configuration, or service. Do not treat it as an in-place upgrade command. Follow the [server guide](server/README.md) for deployment details.

## Connect a device

Generate a separate profile.masque bundle for each device; see [issuing client configurations](docs/CLIENTS.md). Transfer profiles privately.

| Platform | Start here |
|---|---|
| Android | Install the v2.2.0 phone APK, import the profile, grant VPN permission, connect. [Android guide](android/README.md) |
| Android TV | Install the v2.2.0 TV APK. Clipboard/paste-text import is available. [Android guide](android/README.md) |
| Windows | Install the v2.2.0 MSI (one UAC prompt). It replaces the previous service automatically. Open MASQUE VPN, import the profile, connect. Daily use does not require administrator privileges; closing the window leaves the tunnel running. [Windows guide](windows/README.md) |
| iOS | Install iOS 2.2.0 through [TestFlight](https://testflight.apple.com/join/x52N41V1) by 7 December 2026 (the published build stays 2.1.1 until then), then import a device-specific profile. [iOS guide](ios/README.md) |

## Security and limitations

- A private CA signs the server and client certificates. Clients trust that CA; VPN sessions require client authentication. The TCP website does not grant access to the tunnel.
- Never publish private keys, real client profiles, Android keystores, or signing credentials. Distribute bundles out-of-band.
- Back up ca.key securely. Losing it prevents issuing new client certificates under the existing CA.
- Keep insecure = false. Do not disable server verification just to suppress a browser warning.
- Traffic exits at your VPS. This is not anonymous access or a public VPN service.
- Read the [security policy](SECURITY.md). CI builds and tests are not a third-party penetration test.

## Repository layout

| Directory | Contents |
|---|---|
| server/ | Server deployment, systemd, Docker, configuration generator |
| android/ | Android and Android TV clients; shared Go source |
| windows/ | Windows service, GUI, MSI |
| ios/ | Swift client and Packet Tunnel |

The Go server and shared client core live in android/go-src/masque-vpn/ (module github.com/Next1971/masque-vpn).

## Documentation and community

- [Server installation](server/README.md)
- [Client configuration bundles](docs/CLIENTS.md)
- [Wiki](https://github.com/Next1971/masque-vpn/wiki)
- [Roadmap](docs/ROADMAP.md)
- [Censorship resistance and limitations](docs/CENSORSHIP.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Report a bug](../../issues/new/choose)
- [Discussions](../../discussions)
- [Support the project](DONATE.md)

Versioned documentation beside the code is the source of truth for exact commands and configuration formats.

## Support the project

MASQUE VPN is developed independently. If it is useful to you, you can support its development with [Bitcoin or Monero](DONATE.md).
