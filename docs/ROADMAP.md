# Roadmap

> Status snapshot: 2026-10-17. This document covers current release status, project decisions, and future work. See [CHANGELOG.md](../CHANGELOG.md) and [GitHub Releases](../../../releases) for detailed release history. Release notes take precedence over older platform-guide introductions that still describe v1.7.1 as Latest.

## Current releases

| Release | Status | Scope |
|---|---|---|
| [v2.1.0](../../../releases/tag/v2.1.0) | GitHub Latest | Android phone/TV, Windows, Linux server, install.sh, gen-config.sh, checksums, Docker sources, and masque-setup.exe. UDP and TCP CONNECT-IP; TCP serves an HTML page to connections without a valid client certificate. |
| [v2.1.1](../../../releases/tag/v2.1.1) | Optional pre-release, not Latest | Android TV APK only, for sets where the v2.1.0 Connect button is below the screen. No phone APK, MSI, or server binary. |
| iOS 2.0 | Current TestFlight client | Public [join link](https://testflight.apple.com/join/x52N41V1). Uses the private CA and connects over UDP only. TCP does not carry traffic on iOS yet. No GitHub IPA. |
| [v2.0.0](../../../releases/tag/v2.0.0) | Earlier experimental pre-release | First TCP CONNECT-IP experiment; fixed client MTU 1340 and automatic fallback after an unanswered UDP probe. TCP required a client certificate at the handshake and did not serve a website. Not Latest. |
| [v1.7.1](../../../releases/tag/v1.7.1) | Previous Latest | Android phone/TV and Windows clients; UDP-only. No server binary in that release. |
| [v1.7](../../../releases/tag/v1.7) | Older iOS TestFlight release | Apple build 20, UDP-only. Its 4 December 2026 expiry refers to that older build, not to the current iOS 2.0 build. |
| [v1.5.6](../../../releases/tag/v1.5.6) | Earlier release | IPv6 QUIC transport, dual-stack server bind, Windows Disconnect fix, and server installation packaging. |

All releases remain experimental software. GitHub build/test checks are not an independent security audit. A new Latest release does not guarantee suitability for every network or deployment.

## v2.1.0: delivered

### Transport, authentication, and client controls

- HTTP/3 CONNECT-IP over QUIC/UDP remains the normal tunnel transport.
- The server also listens for HTTP/2 CONNECT-IP over TCP on the same port number.
- On UDP, a valid client certificate from the private CA is required.
- On TCP, the TLS handshake requests but does not require a client certificate. A valid client certificate and an HTTP/2 CONNECT-IP request are required to open the VPN. Browsers, missing certificates, and invalid certificates receive an HTML page rather than tunnel access.
- The server certificate is still signed by the private CA. A browser shows a certificate warning before the page. UDP does not serve a website.
- Android phone, Android TV, and Windows expose Automatic / UDP only / TCP only transport selection and a configurable MTU with default 1340.
- These clients also expose a kill switch (off by default), day/month traffic statistics, and ping on TCP sessions. QUIC ping remains the smoothed QUIC RTT; TCP ping must not be described as QUIC RTT.
- iOS 2.0 is available through TestFlight and is UDP only for now. TCP does not carry traffic on iOS.

### Compatibility and deployment

- Existing profiles and certificates do not change. An existing v2.0.0 server can be replaced with the v2.1.0 binary without issuing new client bundles.
- Older UDP clients, including v1.7.1, still connect to a v2.1.0 server over UDP.
- v2.0.0 and v2.1.0 clients can connect to the v2.1.0 server over UDP and TCP. v2.1.0 clients can still use older servers over UDP; TCP requires a v2.0.0 or v2.1.0 server.
- Linux amd64/arm64 server binaries, install.sh, gen-config.sh, SHA256SUMS, and Docker sources are included.
- install.sh verifies SHA256SUMS and refuses to overwrite an existing CA, configuration, or service. It is a fresh-install path, not an in-place upgrade command.
- masque-setup.exe is included in v2.1.0. With the Linux binary beside it, the Windows wizard deploys UDP and TCP, supports an optional second port, and displays a profile QR code.
- Docker sources are in masque-docker-2.1.0.zip; image tag masque-vpn-server:2.1.0. Host networking, NET_ADMIN, and /dev/net/tun are required. No image tarball is attached.

See the [v2.1.0 release notes](../../../releases/tag/v2.1.0) for files and deployment details.

## MTU observations and earlier experiment

The v2.0.0 experiment forced client MTU 1340 and did not expose manual transport or MTU controls. Those restrictions are historical, not the v2.1.0 interface.

A phone-path sweep on 2026-10-11 found UDP usable at MTU 1280 and 1340, slow at 1359, and unusable at 1365 and 1369. TCP remained usable at every tested MTU and was fastest at 1280. These are observations from that tested path, not universal protocol rankings or a promise that TCP is always faster. See [benchmarks](benchmarks/) for supporting measurements.

## Planned work

No committed release numbers or dates are assigned to the items below.

- [ ] Expand troubleshooting documentation and platform-specific FAQs; remove stale release-status and transport descriptions from server and client guides and Wiki pages.
- [ ] Add or complete current-session upload/download counters across Android phone/TV, Windows, and iOS, separate from the day/month statistics delivered on Android/TV/Windows. The original proposal counts tunnel IP packets in the shared pump and resets the session totals on Disconnect; confirm existing implementation coverage before changing it. No server or protocol change is intended.
- [ ] Add user-selected app exclusions on Android phone using VpnService.Builder.addDisallowedApplication. The MASQUE package itself remains excluded. Apply changes on the next Connect. Android TV is not included; excluded apps are outside the tunnel and its kill switch.
- [ ] Complete and document end-to-end QR profile import for phone clients. The v2.1.0 Windows installer already displays a profile QR code; this does not by itself confirm scanner support on every phone client. TV and Windows retain file/paste import.
- [ ] Maintain iOS TestFlight distribution and publish replacement builds before their actual expiry dates. Record the current build number and expiry from TestFlight; do not reuse the older v1.7 build's date for iOS 2.0.

Client and installer design work may happen in any branch and is not tied to a version number. Play / F-Droid listing work follows the agreed visual snapshot; no publication date is committed here.

## Delivered foundation

This is a capability summary, not a second changelog.

| Area | Delivered capabilities |
|---|---|
| Core tunnel | HTTP/3 CONNECT-IP over QUIC/UDP, HTTP/2 CONNECT-IP over TCP, private-CA client authentication, and a shared Go client core. |
| Server | Config/certificate generator, systemd/NAT setup, multi-client return-path demultiplexing, sticky addresses by client certificate CN, graceful shutdown, optional IPv6 pools/forwarding, and a TCP website for unauthenticated/non-VPN connections. |
| Android and TV | Server-assigned addresses, two-phase connection setup, TV flavor and paste import, network-change reconnect without closing TUN, battery-exemption prompt on phones, optional kill switch, IPv6 handling, transport/MTU controls, and day/month traffic statistics. |
| Windows | Service + GUI + MSI, Wintun, optional kill switch, IPv6 tunnel routes, immediate QUIC session closure on Disconnect, transport/MTU controls, and day/month traffic statistics. |
| Connectivity | QUIC keepalive, optional dual-port QUIC dialing, IPv6 QUIC transport, TCP CONNECT-IP, and documented alternate-port redirects. |
| Operations | Experimental Windows VPS installer with certificate issuance/revocation, UDP/TCP deployment and profile QR display; server CN denylist; install.sh; checksums/attestations; Docker/Compose sources. |
| User interface | Android/Windows icons, version display, QUIC RTT display, and TCP-session ping in v2.1.0 Android/TV/Windows clients. |
| iOS | Public TestFlight distribution with a Packet Tunnel client and shared Go core. iOS 2.0 is UDP only for now. |

The initial public v1.0 release shipped on 14 August 2026. Detailed version-by-version changes belong in the [changelog](../CHANGELOG.md).

## Dependency policy

quic-go and connect-ip-go versions are pinned deliberately. Their APIs and release schedules do not always move together. Dependency upgrades require end-to-end testing across the affected clients and server before adoption; historical upgrade tasks are not future release commitments.

## Out of scope for now

- Additional numbered-profile UI or rename work. Issuance already writes masque-client-N.profile.masque with CN masque-client-N; revocation uses that index. Bootstrap remains profile.masque.
- DoH/DoT inside the tunnel. This is a self-hosted VPS model; plaintext DNS carried inside the tunnel is visible to the VPS operator by design, rather than to the local ISP on the client-to-VPS path.
- GUI certificate management beyond the experimental Windows VPS installer. Credentials are generated and distributed out of band.
- Multi-hop or chained proxy support.
