# Roadmap

> Status snapshot: last updated 2026-08-21. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** across all three components (server, Windows client, Android client). **v1.0** was publicly released on **August 14, 2026**. **v1.3** (21 August 2026) is the Android stability release: keepalive, reconnect, Wi-Fi ↔ LTE, sleep/resume, and sticky tunnel IPs.

| Component | Status | Notes |
|---|---|---|
| Server | Stable | QUIC keepalive, sticky `/32` per client cert CN, mTLS, systemd, cert generator |
| Windows client | Stable | Signed EXE + Wintun DLL in release, console + local web UI, same QUIC keepalive |
| Android client | Stable | Phone + TV flavors; reconnect without tearing TUN; underlying-network callback |

This is experimental software and has not received an independent security audit.

## Completed (v1.0)

- QUIC + HTTP/3 transport via `quic-go`.
- CONNECT-IP (RFC 9484) tunneling via `connect-ip-go`.
- Mutual TLS with an internal EC (P-256) CA.
- Server config/certificate generator (`gen-config.sh`) producing ready-to-use client bundles.
- Windows client: signed EXE + Wintun DLL, console mode + local web UI.
- Android client: signed APK (all ABIs), Kotlin app + shared Go core (`clientcore`) via gomobile.
- systemd service unit with IP forwarding and NAT.
- CONTRIBUTING.md, SECURITY.md, issue templates.

## Completed (v1.2)

- Server multi-client return-path demux by destination IP.
- Android two-phase connect using the server-assigned `/32`.
- Android TV product flavor (leanback, paste-text import).

## Completed (v1.3)

- QUIC keepalive and idle timeout on client and server.
- Android battery-optimization exemption prompt.
- Reconnect in the gomobile bridge without closing the TUN.
- `NetworkCallback` + `setUnderlyingNetworks` + UDP `protect` for Wi-Fi → cellular.
- Sticky IP pool keyed by client certificate CN.
- `poc-client` requires a CA (no default `InsecureSkipVerify`).

## Known limitations (all platforms)

- IPv4 only inside the tunnel (no IPv6 support yet).
- In-tunnel DNS is plaintext UDP:53 — hidden from the local ISP but visible to the server operator. DoH/DoT is planned.
- Single server/profile per client — no profile list or automatic failover.
- No independent security audit yet.
- Some OEM battery savers ignore the exemption dialog; a killed process still needs a manual Connect.

## In progress / next (v1.x remainder)

- [ ] Automatic recovery after a long airplane-mode stretch (beyond reconnect-on-wake).
- [ ] DNS over HTTPS/TLS (DoH/DoT) inside the tunnel.
- [ ] MTU experiments for different networks.
- [ ] Full Windows desktop client (beyond console + local web UI).
- [ ] Dependency review process for quic-go / connect-ip-go version pinning (see notes below).

## v2.0 (planned)

- [ ] IPv6 support.
- [ ] Coordinated Android Gradle Plugin 9.x + Gradle 9.x migration (not via unattended Dependabot majors).

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Planned for future releases (no committed dates)

- [ ] iOS client.
- [ ] Expanded troubleshooting and platform-specific FAQ.

## Explicitly out of scope for now

- GUI-based certificate management (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
