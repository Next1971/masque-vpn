# Censorship resistance and limitations

This document describes how MASQUE VPN's transport interacts with known network
censorship mechanisms, and what it does not do. It is a technical note, not a
guarantee. MASQUE VPN is experimental software and does not promise to bypass
any particular firewall.

## Summary

- The tunnel is standard HTTP/3 CONNECT-IP (RFC 9484) over QUIC, with HTTP/2
  over TCP as a fallback on the same port number.
- The project uses no custom obfuscation layer and no active attacks against
  censorship systems.
- Whether it works on a given network depends on that network's blocking
  policy, your VPS address, and the port you choose. Test on your own path.

## Background: QUIC SNI censorship

Research from GFW Report, published at USENIX Security 2025, shows that the
Great Firewall of China can decrypt QUIC Initial packets, read the server name
(SNI), and block connections to blacklisted domains regardless of the server
IP address. The paper also describes weaknesses in that system, including
limited handling of fragmented Client Hellos and of unusual flow patterns.

Paper and artifacts:
[Exposing and Circumventing SNI-based QUIC Censorship of the Great Firewall of China](https://gfw.report/publications/usenixsecurity25/en/).

Other networks may use different methods, such as IP or port blocking, UDP
throttling, or blocking all QUIC. These are not covered by that research.

## How MASQUE VPN is built

- **QUIC and HTTP/3** through `quic-go`.
- **CONNECT-IP** through `connect-ip-go`.
- **Mutual TLS** with a private CA. Every VPN session needs a valid client
  certificate.
- **Transport selection** on Android, Android TV, and Windows:
  Automatic, UDP only, or TCP only. Automatic falls back to TCP when UDP is
  not answered.
- **Configurable MTU** (default 1340). Some paths drop larger UDP packets; see
  the [roadmap](ROADMAP.md) for one measured example.
- **Optional alternate port** and documented port redirects.

The traffic is ordinary QUIC/HTTP/3 or HTTP/2 over TLS. That may make it harder
to distinguish by protocol alone than a dedicated VPN handshake, but this has
not been independently verified, and a network that blocks all QUIC or all
traffic to your VPS will block it.

## What the server reveals

Be aware of these properties when choosing a deployment:

- The server certificate is signed by your private CA, not by a public one. A
  browser opening the TCP port sees a certificate warning, then an HTML page.
  An observer who probes the server can tell it is not an ordinary public
  website.
- UDP does not serve a website. Without a valid client certificate there is no
  VPN access.
- Your VPS IP address is visible to the network between you and the server.
  If that address is blocked, the tunnel will not connect.
- DNS inside the tunnel is visible to the VPS operator by design.

## Mitigations you can try

- Switch transport to TCP only, or leave it on Automatic, if UDP is throttled.
- Run the server on a different port, or enable the optional second port.
- Lower MTU (for example 1280) if UDP is slow or unstable on your path.
- Use a fresh VPS address if the current one is blocked.

None of these is guaranteed to work.

## Out of scope

MASQUE VPN does not:

- implement active attacks such as flooding, degradation, or other availability
  attacks on censorship equipment;
- use packet-level tricks that depend on bugs in a specific censor;
- provide anonymity. Traffic exits at your VPS, which you operate;
- provide domain fronting, traffic shaping to imitate a specific website, or
  multi-hop chaining.

Censorship techniques change. A transport that works today may stop working.

## Reporting results

Reports of what works and what does not on specific networks are useful.
Please open an issue with the country or network type, transport (UDP/TCP),
port, MTU, client version, and server version. Do not include private keys,
profiles, or full IP addresses of other people.

## See also

- [Roadmap](ROADMAP.md)
- [Security policy](../SECURITY.md)
- [Server guide](../server/README.md)
