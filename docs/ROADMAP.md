# Roadmap

> Status snapshot: last updated 2026-08-15. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** across all three components (server, Windows client, Android client). **v1.0** was publicly released on **August 14, 2026**, with a signed Android APK and a Windows EXE bundle attached to the release.

| Component | Status | Notes |
|---|---|---|
| Server | Stable | QUIC + HTTP/3 CONNECT-IP, mTLS, systemd unit, cert generator |
| Windows client | Stable | Signed EXE + Wintun DLL in release, console + local web UI |
| Android client | Stable | Signed APK (all ABIs) in release, Kotlin app + Go core via gomobile |

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

## Known limitations (all platforms)

- IPv4 only inside the tunnel (no IPv6 support yet).
- In-tunnel DNS is plaintext UDP:53 — hidden from the local ISP but visible to the server operator. DoH/DoT is planned.
- Single server/profile per client — no profile list or automatic failover.
- No independent security audit yet.

## In progress / next release

- [ ] DNS over HTTPS/TLS (DoH/DoT) inside the tunnel.
- [ ] IPv6 support.
- [ ] Multiple server profiles per client.
- [ ] Dependency review process for quic-go / connect-ip-go version pinning (see notes below).

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Planned for future releases (no committed dates)

- [ ] iOS client.
- [ ] Android TV client.
- [ ] Expanded troubleshooting and platform-specific FAQ.

## Explicitly out of scope for now

- GUI-based certificate management (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
