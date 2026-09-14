# Roadmap

> Status snapshot: last updated 2026-09-14. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** (server, Windows, Android). **v1.0** shipped **14 August 2026**. **v1.5.3** is GitHub **Latest** (daily driver). **v1.5.6** is an optional full release (QUIC to the server over IPv6 + Windows Disconnect fix); previous versions stay compatible over IPv4 — skip it if everything already works. **v1.5.4** is a technical pre-release (`connect-ip-go` v0.3.0 + dual-port). **v1.5.1** and **v1.5.2** remain technical pre-releases. **v1.5.0** was Latest until v1.5.3.

**v1.7** (iOS TestFlight) is scheduled **not later than 12 October 2026**. Client/installer **design work can happen in any branch** and is not gated on a version number. **Store listings** (Play / F-Droid) start **after** that visual snapshot, not in 1.5.x.

| Component | Status | Notes |
|---|---|---|
| Server | Stable | v1.5.3 Latest uses the v1.5.1 binary. **v1.5.6** optional dual-stack `[::]` + IPv6 QUIC. **v1.5.4** pre-release ships `connect-ip-go` v0.3.0 |
| Windows client | Stable | Latest v1.5.3; **v1.5.6** optional (IPv6 QUIC + Disconnect fix); **v1.5.4** pre-release adds dual-port |
| Windows VPS installer | **Experimental (v1.5.6)** | `masque-setup.exe` one UDP port, IPv4/DNS host; not required if the server already runs |
| Android client | Stable | Latest v1.5.3; **v1.5.6** optional IPv6 QUIC; **v1.5.4** pre-release adds dual-port |
| iOS client | **In progress (v1.7)** | Source in `ios/`; TestFlight planned for v1.7 |

This is experimental software and has not received an independent security audit.

## Release slices (near term)

| Tag | Focus | Notes |
|---|---|---|
| **v1.5.3** | Latest: kill switch + TV Connect fix | Client-only vs v1.5.0. Default kill switch **off**. Same protocol. |
| **v1.5.4** | Pre-release: `connect-ip-go` v0.3.0 + dual-port | New server binary. Clients race `[server]` and optional `alt_port`. DoH not in this tag. |
| **v1.5.5** | Packaging (not a GitHub tag) | `install.sh`, SHA256SUMS, Sigstore. Folded into **v1.5.6**. Latest stays v1.5.3. |
| **(after v1.5.5)** | Android AGP/Gradle bump | Reopen deferred Dependabot [#75](https://github.com/Next1971/masque-vpn/pull/75): AGP **9.4.0** + Gradle wrapper **9.6.0**. Soak on main; **no GitHub pre-release**. |
| **v1.5.6** | Optional: QUIC to the server over IPv6 | Full GitHub release, **not** Latest. AAAA + host-route. Windows Disconnect fix. Skip if 1.5.3 already works. |
| **v1.7** | iOS TestFlight | UI refresh ships in whichever build is ready; stores follow the designed UI. |
| **v1.8** | Phone QR profile import | Installer shows a QR; **phone** clients scan it. TV and Windows stay on file/paste. |

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

## Completed (v1.5.1)

- [x] CN denylist on the server (`/opt/masque/blocked_cns`) and revoke in `masque-setup.exe`.
- [x] Default TUN MTU **1369** from path MTU tests (`docs/benchmarks/mtu.md`).
- [x] Documented VPS port redirect when UDP 443 is unusable on the client path.

## Completed (v1.5.2)

- [x] Optional kill switch on Android (`setBlocking` + held TUN) and Windows (routes before dial, held until Disconnect). Default off.

## Completed (v1.5.3)

- [x] Android TV Connect crash: leanback notification intent, skip battery-exemption on TV, IPv6 TUN optional if the device rejects it.
- [x] GitHub **Latest** now includes kill switch + IPv6 tunnel clients (phone, TV, Windows).

## Completed (v1.5.4)

- [x] `connect-ip-go` **v0.3.0** on server and clients (`quic-go` remains v0.62.0).
- [x] Dual-port QUIC dial via optional `[server].alt_port` (any second UDP port; first handshake wins).

## Known limitations (all platforms)

- Connecting to the VPN server on **Latest (v1.5.3)** is still **IPv4 QUIC**. **v1.5.6** adds AAAA / UDP 443 on IPv6 (optional). Do not publish AAAA for names still used by pre-1.5.6 clients.
- Some networks drop outbound **UDP 443**. Use an alternate UDP port with VPS DNAT (see [server README](../server/README.md#udp-443-blocked-on-the-client-path)). **v1.5.4** clients race the profile port and optional `alt_port`.
- In-tunnel DNS is plaintext UDP:53 — hidden from the local ISP but visible to the server operator. DoH/DoT is **not planned**: the project targets a **VPS you operate**, so that visibility is not treated as a product hole.
- Kill switch (v1.5.2+) does not survive a killed VPN process. On Android, system Always-on VPN / “Block connections without VPN” is the extra layer.
- Single server/profile per client — no profile list or automatic failover.
- No independent security audit yet.
- Some OEM battery savers ignore the exemption dialog; a killed process still needs a manual Connect.
- NAT64/DNS64 is not included: AAAA destinations need WAN IPv6 on the VPS.
- CN denylist is not a CRL/OCSP PKI: it is a server-side name list reloaded on process start.

## Completed (v1.5.5)

- [x] Server `install.sh`, SHA256SUMS, Sigstore attestations (shipped in the **v1.5.6** GitHub release; no separate v1.5.5 tag).

## Completed (v1.5.6)

- [x] QUIC to the server over IPv6 (AAAA + host-route bypass). Dual-port stayed in v1.5.4.
- [x] Windows **Disconnect** closes the QUIC session immediately.
- [x] GitHub full release with MSI, APKs, setup, Linux server, `install.sh`. Latest remains v1.5.3.

## After v1.5.6 (no separate tag)

- [ ] Android toolchain: AGP **9.3.2 → 9.4.0** with Gradle wrapper **9.5.0 → 9.6.0** (deferred [#75](https://github.com/Next1971/masque-vpn/pull/75)). Merge and test phone/TV builds; do not cut a pre-release for this bump.

## Planned for v1.7 (not later than 12 October 2026)

- [ ] iOS TestFlight access (source already in `ios/`; first on-device connect succeeded; no GitHub IPA in 1.5.x).
- Visual refresh of clients and `masque-setup.exe` can merge whenever it is ready. **App stores after that UI**, not before.

## Planned for v1.8

- [ ] Phone QR instead of (or in addition to) a profile file: after Issue, `masque-setup.exe` shows a QR of the bundle. **Android phone** (and iOS once TestFlight exists) scan it in-app. Not for Android TV or Windows (no camera / not the path). File save remains. Payload is the existing `profile.masque` (private key in the QR — treat like the file). Compact encoding if a raw TOML QR is too dense.

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Planned for future releases (no committed dates)

- [ ] Expanded troubleshooting and platform-specific FAQ.

## Explicitly out of scope for now

- Extra numbered-profile work (`masque-client-N.masque` rename, index in the client UI). Issue already writes `masque-client-N.profile.masque` with CN `masque-client-N`; revoke uses that `N`. Bootstrap stays `profile.masque`.
- DoH/DoT inside the tunnel. MASQUE is aimed at a **self-hosted VPS**; plaintext UDP:53 in the tunnel is visible to the operator by design, not to the local ISP.
- GUI-based certificate management beyond the experimental Windows VPS installer (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
