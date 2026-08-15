# Censorship and QUIC SNI

This document briefly summarizes how MASQUE VPN's transport stack interacts with
known censorship mechanisms, in particular the Great Firewall of China (GFW)
and its QUIC SNI–based blocking.

## Background: GFW QUIC SNI censorship

Recent research by GFW Report, published at USENIX Security 2025, shows that the
GFW performs **SNI-based censorship on QUIC client Initial packets** — it
decrypts the Initial, extracts the server name (SNI), and blocks connections to
domains on its blacklist regardless of the server IP address.[web:169][web:170]

The same research reverse-engineered the GFW’s heuristics and found several
implementation bugs:

- It only tracks flows where the **source port is greater than the destination
  port**. When a server listens on a high UDP port (e.g. 65535) and clients use
  a lower source port, the flow is not inspected by the SNI censor.[web:170]
- It does not correctly **reassemble fragmented Client Hellos** across multiple
  QUIC CRYPTO frames in different datagrams, so carefully slicing the SNI
  across frames can prevent the censor from parsing it.[web:169]
- A single **random UDP packet sent before the real QUIC Initial** confuses the
  flow-tracking logic, causing the censor to ignore the subsequent Initial
  packet.[web:169]
- Under load, the decryption and inspection logic is vulnerable to a
  **degradation attack**: flooding it with QUIC Initials can overwhelm its
  capacity and let a large fraction of otherwise-blocked connections through
  during daytime traffic.[web:170][web:173]

These behaviors are documented in the paper “Exposing and Circumventing
SNI-based QUIC Censorship of the Great Firewall of China” and its public code
artifacts.[web:169][web:168]

## Relevance to MASQUE VPN

MASQUE VPN uses:

- **QUIC + HTTP/3** via `quic-go`.
- **CONNECT-IP** (RFC 9484) via `connect-ip-go`.
- Mutual TLS (mTLS) with an internal EC (P-256) CA.[cite:182]

The project deliberately builds on a modern QUIC stack rather than legacy VPN
protocols (such as OpenVPN or WireGuard in their default modes), because the
research indicates that those protocols are now more easily fingerprinted and
blocked compared to HTTP/3-based tunnels.[web:209]

In particular:

- Recent versions of **`quic-go` include SNI-slicing mitigations** described in
the GFW Report paper, reducing exposure to naive SNI-based QUIC censorship
without requiring custom patches in this project.[web:169][web:173]
- The MASQUE VPN transport is designed to **blend into ordinary HTTP/3 traffic**
(QUIC + CONNECT-IP), which is more likely to survive generic protocol-level
blocking than bespoke VPN handshakes.

## Scope and limitations

This project:

- **Does not implement active attacks against censorship systems** (such as
  degradation or availability attacks) — it only uses a robust QUIC/HTTP3
  transport.[web:169][web:210]
- Does not claim to “guarantee bypass” of any specific national firewall.
  Censorship techniques evolve, and no single design can promise permanent
  circumvention.
- Focuses on correct, secure tunneling (mTLS, sanitized public server config)
  rather than on offensive use of known censorship bugs.

For readers who want deeper technical details, see:

- USENIX Security 2025 paper and artifacts:
  https://gfw.report/publications/usenixsecurity25/en/[web:169][web:168]
- Follow-up analyses and blog posts summarizing QUIC SNI censorship behavior
  and its practical implications.[web:170][web:173][web:209]
