# Roadmap

> Status snapshot: 2026-10-17. This document covers current release status, project decisions, and future work. See [CHANGELOG.md](../CHANGELOG.md) and [GitHub Releases](../../../releases) for detailed release history.

## Current releases

| Release | Status | Scope |
|---|---|---|
| [v2.1.0](../../../releases/tag/v2.1.0) | GitHub Latest | Phone APK, TV APK, Windows MSI, `masque-setup.exe`, Linux server, `install.sh`, and Docker sources. UDP is the normal tunnel. TCP on the same port opens the VPN for a valid client certificate and serves a website otherwise. [iOS 2.0](https://testflight.apple.com/join/x52N41V1) is the TestFlight client. No IPA. |
| [v2.0.0](../../../releases/tag/v2.0.0) | Experimental pre-release | First lab build with a TCP listener. It still required a client certificate, so it did not serve a site. Not Latest. |
| [v1.7.1](../../../releases/tag/v1.7.1) | Previous Latest | Android and Windows only, UDP only. No server binary in that drop. |
| [v1.7](../../../releases/tag/v1.7) | Older iOS TestFlight | Apple build 20, UDP only. Superseded on TestFlight by iOS 2.0. |
| [v1.5.6](../../../releases/tag/v1.5.6) | Optional earlier release | IPv6 QUIC transport, dual-stack server bind, Windows Disconnect fix, and server installation packaging. |

A 1.7.1 client still reaches a 2.1.0 server over UDP. TCP needs a 2.0.0 or 2.1.0 server. Profiles do not change.

All releases remain experimental software. GitHub build/test checks are not an independent security audit.

## v2.1.0: shipped

- TCP asks for a client certificate and does not require one. A certificate signed by the private CA, plus an HTTP/2 CONNECT-IP request to `/vpn`, opens the tunnel. A missing, foreign, or revoked certificate gets an HTML page. UDP still requires the private CA and has no website.
- Client settings on Android phone, Android TV, Windows, and iOS: transport (Automatic, UDP only, or TCP only), MTU (default 1340, range 1280–1500), kill switch (default off).
- Day and month traffic on the main screen, in MB. Counted in the shared pump and kept across reconnects and process restarts.
- Ping during a TCP session (HTTP/2 PING), shown the same way as the QUIC RTT.
- `masque-setup.exe` installs the UDP+TCP server, an optional second port, and shows a profile QR after Issue and after saving the bootstrap profile. Android phone and iOS scan it. Android TV and Windows keep file import.
- iOS 2.0 on TestFlight tries the profile UDP port, then `alt_port`, then TCP on both. A hostname with AAAA is dialed over IPv6. No GitHub IPA.

## v2.0.0: shipped experiment

### Transport and MTU decisions

- Android phone, Android TV, and Windows forced tunnel MTU 1340. That release had no user-facing MTU field or TCP/UDP switch. v2.1.0 added both in Settings.
- UDP remains the normal transport. HTTP/2 CONNECT-IP over TCP starts only when a short UDP probe receives no reply; this is not automatic selection of the fastest transport.
- TCP uses the same host and profile port, followed by the optional `alt_port`.
- One server process listens on UDP and TCP on the same bind port. Existing profiles and certificates do not change.
- The Android `1.8.0-tcp` build with manual transport and MTU controls is a lab APK, not the v2.0.0 UI.

These decisions followed a phone-path sweep on 2026-10-11: UDP was usable at MTU 1280 and 1340, slow at 1359, and unusable at 1365 and 1369. TCP remained usable at every tested MTU and was fastest at 1280. These are observations from that tested path, not universal protocol rankings.

### Compatibility and deployment

- Older UDP clients, including v1.7.1, can connect to the v2.0.0 server.
- v2.0.0 clients can connect to older servers over UDP. TCP fallback requires a v2.0.0 or v2.1.0 server.
- iOS TestFlight v1.7 stayed UDP-only. iOS 2.0 speaks UDP and TCP.
- Linux amd64/arm64 server binaries, `install.sh`, `gen-config.sh`, checksums, and Docker sources are included.
- `masque-setup.exe` was not in v2.0.0. It ships in v2.1.0 and opens UDP and TCP.
- Server TUN MTU remains 1369; client MTU default is 1340.
- Losing UDP probes no longer remain blocked after the first answer or cancellation of Connect.

See the [v2.0.0 release notes](../../../releases/tag/v2.0.0) and the [v2.1.0 release notes](../../../releases/tag/v2.1.0).

## Planned work

No committed release numbers or dates are assigned to the items below.

- [ ] Expand troubleshooting documentation and platform-specific FAQs.
- [ ] Add user-selected app exclusions on Android phone using `VpnService.Builder.addDisallowedApplication`. The MASQUE package itself remains excluded. Apply changes on the next Connect. Android TV is not included; excluded apps are outside the tunnel and its kill switch.
- [ ] Accept an IPv6 literal in the Public host field of `masque-setup.exe`. Today that value is still manual (`MASQUE_DIAL` / `gen-config.sh --dial`). The wizard accepts IPv4 or a DNS name.

Client and installer design work may happen in any branch and is not tied to a version number. Play / F-Droid listing work follows the agreed visual snapshot; no publication date is committed here.

## Future option: TCP 443 next to Caddy

Not scheduled. Recorded 2026-10-11. v2.1.0 already serves the cover page from MASQUE itself on the TCP listen port. The certificate is still the private CA, so a browser warns before the page. Sharing that port with Caddy, and presenting a Let's Encrypt certificate, is a later step.

Caddy on a production VPS already owns TCP 443: one cover site plus camouflage copies on their own names (masque, trust, naive). An HTTP reverse proxy in front of MASQUE does not work: the client does its own TLS, and the tunnel is HTTP/2 CONNECT-IP (`:protocol connect-ip`, path `/vpn`).

If that handoff is built later, the server certificate everyone sees is Let's Encrypt. A valid profile trusts that chain. The client key stays issued by the private CA. A valid client certificate opens the CONNECT-IP tunnel. v2.1.0 already decides after the handshake: the TCP listener asks for a client certificate and does not reject the handshake when it is missing.

- **Option A.** Any other client gets the camouflage site for the masque name. The page can be served locally or proxied to that vhost. Caddy stops terminating TLS on this name. MASQUE reads the Let's Encrypt files.
- **Option B.** Any other client is switched to the main cover site, not the masque camouflage copy.

UDP 443 stays a separate socket. This does not share it with Caddy HTTP/3 or TrustTunnel.

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
