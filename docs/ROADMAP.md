# Roadmap

> Status snapshot: last updated 2026-08-28. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** across all three components (server, Windows client, Android client). **v1.0** was publicly released on **August 14, 2026**. **v1.3** is the Android reconnect release. **v1.4** added client polish (icon + on-screen ping). **v1.4.1** (pre-release) is a maintenance line: AGP 9 / Gradle 9, Docker packaging, graceful server shutdown, and Android IPv6-bypass hardening.

| Component | Status | Notes |
|---|---|---|
| Server | Stable | QUIC keepalive, sticky `/32` per CN, mTLS, systemd, optional Docker, graceful SIGTERM |
| Windows client | Stable | Signed EXE + Wintun DLL in release, GUI + tray icon, on-screen ping |
| Android client | Stable | Phone + TV; TUN `/24` + IPv6 sink; version label in UI; reconnect without tearing TUN |

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

## Completed (v1.4)

- App icon on Android and Windows (launcher, tray, MSI, notification, TV banner).
- On-screen ping: smoothed QUIC RTT to the MASQUE server.
- Android airplane-mode soak: after 8 hours offline the tunnel came back cleanly (reconnect + sticky `/32`).

## Completed (v1.4.1)

- Android toolchain: AGP 9.0.1 + Gradle 9.1.0 + built-in Kotlin.
- Docker image / Compose for the server (host network, TUN, NAT).
- Server graceful shutdown on SIGTERM/SIGINT.
- Android IPv6 sink + TUN `/24` (block dual-stack bypass; not full dual-stack VPN).
- Version label in Android UI; high-contrast launcher icon.

## Known limitations (all platforms)

- Tunnel data-plane is still **IPv4-only** (IPv6 is sunk/dropped on Android so apps cannot bypass the VPN; full IPv6-in-tunnel is not shipped).
- In-tunnel DNS is plaintext UDP:53 — hidden from the local ISP but visible to the server operator. DoH/DoT is planned.
- Single server/profile per client — no profile list or automatic failover.
- No independent security audit yet.
- Some OEM battery savers ignore the exemption dialog; a killed process still needs a manual Connect.

## In progress / next (v1.x remainder)

- [ ] DNS over HTTPS/TLS (DoH/DoT) inside the tunnel.
- [ ] MTU experiments for different networks.
- [ ] Dependency review process for quic-go / connect-ip-go version pinning (see notes below).

## v1.5 (planned)

- [ ] Full IPv6 in the tunnel (pool, NAT66/forwarding, client assigned v6).

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Planned for future releases (no committed dates)

- [ ] iOS client.
- [ ] Expanded troubleshooting and platform-specific FAQ.

## Explicitly out of scope for now

- GUI-based certificate management (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
