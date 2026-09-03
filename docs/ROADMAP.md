# Roadmap

> Status snapshot: last updated 2026-09-03. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** across all three components (server, Windows client, Android client). **v1.0** was publicly released on **August 14, 2026**. **v1.3** is the Android reconnect release. **v1.4** added client polish (icon + on-screen ping). **v1.4.1** (pre-release) is a maintenance line: AGP 9 / Gradle 9, Docker packaging, graceful server shutdown, and Android IPv6-bypass hardening. **v1.5.0** adds optional dual-stack inside the tunnel (ULA + NAT66).

| Component | Status | Notes |
|---|---|---|
| Server | Stable | QUIC keepalive, sticky `/32` and optional sticky `/128`, mTLS, systemd, optional Docker, graceful SIGTERM |
| Windows client | Stable | Signed EXE + Wintun DLL in release, GUI + tray icon, on-screen ping, IPv6 default via TUN when the server assigns v6 |
| Windows VPS installer | **Experimental (v1.4.2)** | `masque-setup.exe` pre-release; not a substitute for the documented SSH install |
| Android client | Stable | Phone + TV; dual-stack TUN when the server has a v6 pool; version label in UI; reconnect without tearing TUN |
| iOS client | **In progress** | Packet Tunnel + gomobile; not in a GitHub Release; needs a Mac to compile and a device for TestFlight |

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

- Android toolchain: AGP 9.x + Gradle 9.x + built-in Kotlin (see `android/README.md` for current versions).
- Docker image / Compose for the server (host network, TUN, NAT).
- Server graceful shutdown on SIGTERM/SIGINT.
- Android IPv6 sink + TUN `/24` (block dual-stack bypass on IPv4-only servers).
- Version label in Android UI; high-contrast launcher icon.

## Completed (v1.5.0)

- [x] Full IPv6 in the tunnel (optional pool, NAT66/forwarding, client assigned v6).
- Android forwards assigned IPv6 instead of sinking it when the server has a v6 pool.
- Windows GUI/console and Linux console install IPv6 on TUN and `::/0`.

## Known limitations (all platforms)

- Connecting to the VPN server is still **IPv4 QUIC** (no AAAA / UDP 443 on IPv6 for the control plane in this release).
- In-tunnel DNS is plaintext UDP:53 — hidden from the local ISP but visible to the server operator. DoH/DoT is planned.
- Single server/profile per client — no profile list or automatic failover.
- No independent security audit yet.
- Some OEM battery savers ignore the exemption dialog; a killed process still needs a manual Connect.
- NAT64/DNS64 is not included: AAAA destinations need WAN IPv6 on the VPS.

## In progress / next (v1.x remainder)

- [ ] DNS over HTTPS/TLS (DoH/DoT) inside the tunnel.
- [ ] MTU experiments for different networks.
- [ ] Dependency review process for quic-go / connect-ip-go version pinning (see notes below).
- [ ] QUIC to the server over IPv6 (AAAA + host-route bypass).

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Planned for future releases (no committed dates)

- [ ] iOS client (source tree in `ios/`; TestFlight upload and device soak still open).
- [ ] Expanded troubleshooting and platform-specific FAQ.

## Explicitly out of scope for now

- GUI-based certificate management (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
