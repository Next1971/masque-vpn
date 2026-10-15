# Roadmap

> Status snapshot: last updated 2026-10-15. See [CHANGELOG.md](../CHANGELOG.md) for release history.

## Current status

MASQUE VPN has been operational and tested end-to-end since **July 15, 2026** (server, Windows, Android). **v1.0** shipped **14 August 2026**. **v2.1.0** is GitHub **Latest**. UDP is still the normal tunnel. TCP HTTP/2 CONNECT-IP is on the same port, and anyone who is not a valid client receives a website. That is the first Latest release with either behavior. **iOS 2.0** is [TestFlight](https://testflight.apple.com/join/x52N41V1). **v1.7.1** was the previous Latest (UDP clients only). **v1.5.6** is the optional IPv6-QUIC release.

Client/installer **design work can happen in any branch** and is not gated on a version number. **Store listings** (Play / F-Droid) start **after** that visual snapshot, not in 1.5.x.

| Component | Status | Notes |
|---|---|---|
| Server | Stable | **v2.1.0** Latest. UDP requires the private CA. TCP opens the tunnel for a valid client certificate and serves a page otherwise |
| Windows client | Stable | **v2.1.0** Latest MSI. Settings: transport, MTU, kill switch, day and month traffic |
| Windows VPS installer | **Experimental** | **v2.1.0** `masque-setup.exe`. UDP+TCP listen port, optional second port, profile QR |
| Android client | Stable | **v2.1.0** Latest phone and TV APKs |
| iOS client | **iOS 2.0** | [TestFlight](https://testflight.apple.com/join/x52N41V1). Private CA, UDP and TCP. No GitHub IPA |

This is experimental software and has not received an independent security audit.

## Release slices (near term)

| Tag | Focus | Notes |
|---|---|---|
| **v1.5.3** | Latest: kill switch + TV Connect fix | Client-only vs v1.5.0. Default kill switch **off**. Same protocol. |
| **v1.5.4** | Pre-release: `connect-ip-go` v0.3.0 + dual-port | New server binary. Clients race `[server]` and optional `alt_port`. DoH not in this tag. |
| **v1.5.5** | Packaging (not a GitHub tag) | `install.sh`, SHA256SUMS, Sigstore. Folded into **v1.5.6**. Latest stays v1.5.3. |
| **(after v1.5.5)** | Android AGP/Gradle bump | Reopen deferred Dependabot [#75](https://github.com/Next1971/masque-vpn/pull/75): AGP **9.4.0** + Gradle wrapper **9.6.0**. Soak on main; **no GitHub pre-release**. |
| **v1.5.6** | Optional: QUIC to the server over IPv6 | Full GitHub release, **not** Latest. AAAA + host-route. Windows Disconnect fix. Skip if 1.5.3 already works. |
| **v1.7** | iOS TestFlight (shipped) | Public link; Apple build 20. No GitHub IPA. Latest stays v1.5.3. |
| **v1.8** | Phone QR profile import | Done in the tree. Installer shows a QR; phone and iOS scan it. Ships with **v2.1**. |

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

## Completed (v1.7)

- [x] iOS TestFlight (public [join link](https://testflight.apple.com/join/x52N41V1); Apple build **20**; expires **4 December 2026**, then a new 90-day build; no GitHub IPA). Latest remains v1.5.3.

## After v1.5.6 (no separate tag)

- [x] Android toolchain: AGP **9.3.2 → 9.4.1**. Gradle wrapper is **9.8.0** (already past 9.6.0). Shipped in **v1.7.1**.

## Planned for v1.8

- [x] Phone QR instead of (or in addition to) a profile file: after Issue or saving the bootstrap profile, `masque-setup.exe` shows a QR of that `profile.masque` (private key included; the file is still saved). **Android phone** and **iOS** scan it in-app. Not for Android TV or the Windows client. The binary ships with **v2.1**.

Visual refresh of clients and `masque-setup.exe` can merge whenever it is ready. **App stores after that UI**, not before.

## Dependency notes

- `quic-go` and `connect-ip-go` versions are pinned deliberately; `connect-ip-go` releases historically lag behind `quic-go` API changes (e.g. `http3.ParseCapsule`). Upgrades are tested end-to-end on all three platforms before bumping either dependency.

## Decided for v2.0

Phone sweep on 2026-10-11: UDP is up at MTU 1280 and 1340, already slow at 1359, and dead at 1365 and at the old default 1369. TCP keeps a channel at every tested MTU and is fastest at 1280.

- Tunnel MTU base is **1340**. No MTU control in the client.
- No TCP/UDP switch. UDP is the normal path. TCP HTTP/2 CONNECT-IP opens only when UDP does not answer.
- The Android build `1.8.0-tcp` (manual switch and MTU field) is a lab APK, not the 2.0 UI.
- Published as GitHub pre-release **v2.0.0** (full file set). Superseded by Latest **v2.1.0**.
- `masque-setup.exe` was not in that release. The wizard in the tree now matches this server; the binary is part of **v2.1**.

## Planned for v2.1

- [x] On-screen ping during a TCP session. The tunnel already worked, but the UI stayed on "Ping: —" because only QUIC smoothed RTT was read. TCP now samples HTTP/2 PING and shows the same ping on Android, Windows, and iOS. Put this in the v2.1 release notes.
- [x] `masque-setup.exe` catches up to the 2.0 server. The listen port is UDP and TCP. An optional second port is redirected to it (UDP and TCP) and written as `alt_port`. UFW opens both protocols on both ports. If UDP or TCP is already taken, the wizard asks whether to continue or pick another port. An existing CA, server config, or `masque.service` is left in place (no new CA, no deleted clients, no deleted denylist).
- [x] Profile QR in `masque-setup.exe` after Issue and after saving the bootstrap profile. Android phone and iOS scan it. Android TV and the Windows client keep file import. Put this in the v2.1 release notes.
- [x] Settings on Android phone, Android TV, Windows, and iOS. Transport is Automatic (UDP, then TCP) by default, or UDP only, or TCP only. Kill switch is in Settings on all of them (default off). On iOS it blocks traffic if the tunnel drops, until MASQUE reconnects or the user disconnects. A connect that never came up does not leave that block on. No in-tunnel IPv6 default route. MTU is there too (default 1340, range 1280–1500) and applies on the next connect. Put this in the v2.1 release notes.
- [x] Main screen traffic: bytes up and down since local midnight, and since the first of the month, in MB. Counted in the shared pump and kept across reconnects and process restarts. Put this in the v2.1 release notes.
- [x] iOS 2.0 is the TestFlight client ([join/x52N41V1](https://testflight.apple.com/join/x52N41V1)). It tries the profile UDP port, then `alt_port`, then TCP on both. A hostname with AAAA is dialed over IPv6. In-tunnel IPv6 default route stays off. No GitHub IPA.
- [x] TCP serves a website when the client certificate is missing, foreign, or revoked. A valid certificate and a CONNECT-IP request still open the tunnel. UDP still requires the private CA. Shipped in **v2.1.0**.
- [ ] IPv6 literal in the Public host field is still manual (`MASQUE_DIAL` / `gen-config.sh --dial`). The wizard accepts IPv4 or a DNS name.

## Future option: TCP 443 next to Caddy

Not scheduled. Recorded 2026-10-11. **v2.1.0** already serves the cover page from MASQUE itself on the TCP listen port. The certificate is still the private CA, so a browser warns before the page. Sharing that port with Caddy, and presenting a Let's Encrypt certificate, is a later step.

Caddy on a production VPS already owns TCP 443: one cover site plus camouflage copies on their own names (masque, trust, naive). An HTTP reverse proxy in front of MASQUE does not work: the client does its own TLS, and the tunnel is HTTP/2 CONNECT-IP (`:protocol connect-ip`, path `/vpn`).

If that handoff is built later, the server certificate everyone sees is Let's Encrypt. A valid profile trusts that chain. The client key stays issued by the private CA. A valid client certificate opens the CONNECT-IP tunnel. v2.1.0 already decides after the handshake: the TCP listener asks for a client certificate and does not reject the handshake when it is missing.

- **Option A.** Any other client gets the camouflage site for the masque name. The page can be served locally or proxied to that vhost. Caddy stops terminating TLS on this name. MASQUE reads the Let's Encrypt files.
- **Option B.** Any other client is switched to the main cover site, not the masque camouflage copy.

UDP 443 stays a separate socket. This does not share it with Caddy HTTP/3 or TrustTunnel.

## Planned for future releases (no committed dates)

- [ ] Expanded troubleshooting and platform-specific FAQ.
- [ ] Android phone: user-selected apps excluded from the tunnel (`VpnService.Builder.addDisallowedApplication`). The Masque package itself stays excluded. The list applies on the next Connect. Not for Android TV. Excluded apps leave the tunnel, so they are outside the kill switch.

## Explicitly out of scope for now

- Extra numbered-profile work (`masque-client-N.masque` rename, index in the client UI). Issue already writes `masque-client-N.profile.masque` with CN `masque-client-N`; revoke uses that `N`. Bootstrap stays `profile.masque`.
- DoH/DoT inside the tunnel. MASQUE is aimed at a **self-hosted VPS**; plaintext UDP:53 in the tunnel is visible to the operator by design, not to the local ISP.
- GUI-based certificate management beyond the experimental Windows VPS installer (certs are generated/distributed out-of-band by design).
- Multi-hop / chained proxy support.
