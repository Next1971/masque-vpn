# MASQUE VPN Wiki

MASQUE VPN is an experimental, self-hosted VPN using MASQUE CONNECT-IP with client-certificate authentication. Run the server on your own VPS and issue a separate profile for each device.

Current release: [v2.1.0](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0). Clients are available for Android, Android TV, Windows, and [iOS 2.0 through TestFlight](https://testflight.apple.com/join/x52N41V1). No IPA is attached to GitHub Releases.

## What changed in v2.1.0

- HTTP/3 CONNECT-IP over QUIC/UDP remains the normal transport.
- HTTP/2 CONNECT-IP over TCP uses the same port number. A valid client certificate and a CONNECT-IP request are required for VPN access.
- A browser or a TCP connection without a valid client certificate receives an HTML page, not a VPN tunnel. The private-CA server certificate produces a browser warning. UDP does not serve a website.
- Android, Android TV, and Windows offer Automatic / UDP only / TCP only, configurable MTU (default 1340), kill switch (off by default), day/month traffic statistics, and ping on TCP sessions.
- iOS 2.0 on TestFlight works over UDP and TCP.

## Start here

1. Read the [repository README](https://github.com/Next1971/masque-vpn#readme) and [release notes](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0).
2. Choose an installation path in the [server guide](https://github.com/Next1971/masque-vpn/blob/main/server/README.md): manual SSH, experimental Docker Compose, or the experimental Windows VPS Setup Helper.
3. [Issue a separate configuration bundle for each device](https://github.com/Next1971/masque-vpn/blob/main/docs/CLIENTS.md). Keep profiles and private keys private.
4. Follow the relevant client guide: [Android / Android TV](https://github.com/Next1971/masque-vpn/blob/main/android/README.md), [Windows](https://github.com/Next1971/masque-vpn/blob/main/windows/README.md), or [iOS](https://github.com/Next1971/masque-vpn/blob/main/ios/README.md).

## Compatibility and upgrades

Profiles and certificates do not change in v2.1.0. Older UDP clients, including v1.7.1, still work with the v2.1.0 server over UDP. TCP requires a v2.0.0 or v2.1.0 server. A v2.1.0 client still reaches older servers over UDP. The old iOS 1.7 TestFlight build is UDP-only.

install.sh is a fresh-install path and refuses to overwrite an existing CA, configuration, or service. Read the server documentation before upgrading; do not regenerate the CA just to update the server binary.

## Documentation source of truth

Exact commands, supported options, and configuration formats are maintained beside the code. Use documentation for the release you deploy when it differs from main.

- [Server installation](https://github.com/Next1971/masque-vpn/blob/main/server/README.md)
- [Client profiles](https://github.com/Next1971/masque-vpn/blob/main/docs/CLIENTS.md)
- [Censorship resistance and limitations](https://github.com/Next1971/masque-vpn/blob/main/docs/CENSORSHIP.md)
- [Roadmap](https://github.com/Next1971/masque-vpn/blob/main/docs/ROADMAP.md)
- [Benchmarks](https://github.com/Next1971/masque-vpn/tree/main/docs/benchmarks)
- [Security policy](https://github.com/Next1971/masque-vpn/blob/main/SECURITY.md)
- [Contributing](https://github.com/Next1971/masque-vpn/blob/main/CONTRIBUTING.md)
- [Changelog](https://github.com/Next1971/masque-vpn/blob/main/CHANGELOG.md)

## Wiki navigation

- [[Getting Started]] — prerequisites and links to current installation instructions.
- [[Architecture]] — transport and authentication concepts; consult v2.1.0 release notes for current TCP behavior.
- [[Troubleshooting]] — reachability, certificates, logs, and client checks.
- [[MTU Testing]] — network-path testing and MTU observations.
- [[Security Model]] — trust boundaries, credentials, and operator responsibilities.
- [[FAQ]] — project scope and common questions.

## Before opening an issue

1. Check the latest release notes and the matching server/client guides.
2. Record client and server versions, platform, chosen transport, and MTU.
3. Note whether the issue occurs on UDP, TCP, or both. A visible TCP webpage does not prove VPN authentication or tunnel routing works.
4. Include reproducible steps and relevant logs with secrets removed. Never attach private keys or an unredacted real profile.
5. Use [Issues](https://github.com/Next1971/masque-vpn/issues) for bugs and [Discussions](https://github.com/Next1971/masque-vpn/discussions) for questions and deployment notes. Report security vulnerabilities through the security policy.

## Project status

Experimental software, with no independent security audit. Test in your own environment; do not rely on it as your sole critical connectivity or security solution. Traffic exits at your VPS, and the VPS owner remains responsible for credentials, updates, and deployment security. No guarantee of bypassing a particular firewall.
