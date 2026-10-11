# Roadmap

> Status snapshot: 2026-10-11. This document covers current release status, project decisions, and future work. See [CHANGELOG.md](../CHANGELOG.md) and [GitHub Releases](../../../releases) for detailed release history.

## Current releases

| Release | Status | Scope |
|---|---|---|
| [v1.7.1](../../../releases/tag/v1.7.1) | GitHub Latest | Android phone/TV and Windows clients. Compatible with the existing 1.5.x UDP server; no server upgrade is required for this release. |
| [v2.0.0](../../../releases/tag/v2.0.0) | Experimental pre-release | Android phone/TV, Windows, and Linux server: client MTU 1340 and automatic HTTP/2 CONNECT-IP fallback when the UDP probe receives no reply. Does not replace Latest. |
| [v1.7](../../../releases/tag/v1.7) | iOS TestFlight | Public [join link](https://testflight.apple.com/join/x52N41V1), Apple build 20. UDP-only; unchanged by v2.0.0. Expires 4 December 2026. No GitHub IPA. |
| [v1.5.6](../../../releases/tag/v1.5.6) | Optional earlier release | IPv6 QUIC transport, dual-stack server bind, Windows Disconnect fix, and server installation packaging. |

If your current VPN works, you do not need to install v2.0.0. Use v1.7.1 for the current daily-driver client line.

All releases remain experimental software. GitHub build/test checks are not an independent security audit.

## v2.0.0: shipped experiment

### Transport and MTU decisions

- Android phone, Android TV, and Windows force tunnel MTU 1340. There is no user-facing MTU field or TCP/UDP switch.
- UDP remains the normal transport. HTTP/2 CONNECT-IP over TCP starts only when a short UDP probe receives no reply; this is not automatic selection of the fastest transport.
- TCP uses the same host and profile port, followed by the optional `alt_port`.
- One server process listens on UDP and TCP on the same bind port. Existing profiles and certificates do not change.
- The Android `1.8.0-tcp` build with manual transport and MTU controls is a lab APK, not the v2.0.0 UI.

These decisions followed a phone-path sweep on 2026-10-11: UDP was usable at MTU 1280 and 1340, slow at 1359, and unusable at 1365 and 1369. TCP remained usable at every tested MTU and was fastest at 1280. These are observations from that tested path, not universal protocol rankings.

### Compatibility and deployment

- Older UDP clients, including v1.7.1, can connect to the v2.0.0 server.
- v2.0.0 clients can connect to older servers over UDP. TCP fallback requires the v2.0.0 server.
- iOS TestFlight v1.7 stays UDP-only.
- Linux amd64/arm64 server binaries, `install.sh`, `gen-config.sh`, checksums, and Docker sources are included. Install the new server with `install.sh`, or use the documented Docker build.
- `masque-setup.exe` is not included in v2.0.0: the existing experimental Windows wizard still opens only a UDP port.
- Server TUN MTU remains 1369; v2.0.0 client MTU is 1340.
- Losing UDP probes no longer remain blocked after the first answer or cancellation of Connect.

See the [v2.0.0 release notes](../../../releases/tag/v2.0.0) for files and deployment details.

## Planned work

No committed release numbers or dates are assigned to the items below.

- [ ] Expand troubleshooting documentation and platform-specific FAQs.
- [ ] Add session byte counters to Android phone/TV, Windows, and iOS. Count tunnel IP packets in the shared pump; show upload/download totals for the current connection next to ping and reset on Disconnect. No server or protocol change.
- [ ] Add user-selected app exclusions on Android phone using `VpnService.Builder.addDisallowedApplication`. The MASQUE package itself remains excluded. Apply changes on the next Connect. Android TV is not included; excluded apps are outside the tunnel and its kill switch.
- [ ] Add QR profile import for phone clients, with the installer displaying a QR code. TV and Windows retain file/paste import.
- [ ] Refresh the iOS TestFlight build before the current build expires on 4 December 2026.

Client and installer design work may happen in any branch and is not tied to a version number. Play / F-Droid listing work follows the agreed visual snapshot; no publication date is committed here.

## Delivered foundation

This is a capability summary, not a second changelog.

| Area | Delivered capabilities |
|---|---|
| Core tunnel | QUIC + HTTP/3 CONNECT-IP, mutual TLS with an internal CA, and a shared Go client core. |
| Server | Config/certificate generator, systemd/NAT setup, multi-client return-path demultiplexing, sticky addresses by client certificate CN, graceful shutdown, and optional IPv6 pools/forwarding. |
| Android and TV | Server-assigned addresses, two-phase connection setup, TV flavor and paste import, network-change reconnect without closing TUN, battery-exemption prompt on phones, optional kill switch, and IPv6 handling. |
| Windows | Service + GUI + MSI client, Wintun, optional kill switch, IPv6 tunnel routes, and immediate QUIC session closure on Disconnect. |
| Connectivity | QUIC keepalive, optional dual-port QUIC dialing, IPv6 QUIC transport, and documented alternate UDP-port redirects. |
| Operations | Experimental Windows VPS installer with certificate issuance/revocation, server CN denylist, `install.sh`, checksums/attestations, and Docker/Compose sources. |
| User interface | Android/Windows icons, version display, and QUIC RTT display. |
| iOS | Public TestFlight distribution with a Packet Tunnel client and shared Go core. |

The initial public v1.0 release shipped on 14 August 2026. Detailed version-by-version changes belong in the [changelog](../CHANGELOG.md).

## Dependency policy

`quic-go` and `connect-ip-go` versions are pinned deliberately. Their APIs and release schedules do not always move together. Dependency upgrades require end-to-end testing across the affected clients and server before adoption; historical upgrade tasks are not future release commitments.

## Out of scope for now

- Additional numbered-profile UI or rename work. Issuance already writes `masque-client-N.profile.masque` with CN `masque-client-N`; revocation uses that index. Bootstrap remains `profile.masque`.
- DoH/DoT inside the tunnel. This is a self-hosted VPS model; plaintext DNS carried inside the tunnel is visible to the VPS operator by design, rather than to the local ISP on the client-to-VPS path.
- GUI certificate management beyond the experimental Windows VPS installer. Credentials are generated and distributed out of band.
- Multi-hop or chained proxy support.
