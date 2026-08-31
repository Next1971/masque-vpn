# MASQUE connection MTU test matrix

This document tracks MTU validation for MASQUE CONNECT-IP connections across client platforms and network types.

## Interpretation

- **Largest DF-safe MTU**: highest tested interface MTU for which the DF/no-fragment probe succeeds.
- **Connect**: tunnel connection is established successfully.
- **Ping**: baseline ICMP test succeeds without observed packet loss.
- **HTTP/Curl**: HTTP download completes successfully.
- A successful HTTP download does not override a failed DF probe. DF failure is recorded as a possible PMTU/fragmentation risk.

## Summary

| Status | Date | Client platform | Device / OS | Network | IP stack | Server | MTU range | Largest DF-safe MTU | Candidate MTU | Result | Notes |
|---|---|---|---|---|---|---|---|---:|---:|---|---|
| Done | 2026-08-31 | Linux | Linux client | Current test network | Not recorded | MASQUE server | 1280-1500 | **1400** | 1400 | Preliminary pass | DF loss begins at MTU 1420 |
| Planned | - | Android | Android device | Wi-Fi | - | MASQUE server | 1280-1500 | - | - | Pending | Add device model, Android version, and Wi-Fi details |
| Planned | - | Android | Android device | Mobile data | - | MASQUE server | 1280-1500 | - | - | Pending | Add carrier, LTE/5G, and IPv4/IPv6 details |
| Planned | - | Windows | Windows client | Wi-Fi or Ethernet | - | MASQUE server | 1280-1500 | - | - | Pending | Add Windows version and adapter type |

## Linux client to MASQUE server

| MTU | Connect | Connect time, s | Ping | Ping average, ms | DF probe | DF payload | HTTP status | Download speed, B/s | Download time, s | Notes |
|---:|---|---:|---|---:|---|---:|---:|---:|---:|---|
| 1280 | Pass | 1.01 | Pass | 27.291 | Pass | 1252 | 200 | 2,010,869 | 5.215 | |
| 1300 | Pass | 1.02 | Pass | 33.897 | Pass | 1272 | 200 | 2,237,110 | 4.687 | |
| 1350 | Pass | 1.01 | Pass | 38.398 | Pass | 1322 | 200 | 1,589,259 | 6.598 | |
| 1380 | Pass | 1.01 | Pass | 35.568 | Pass | 1352 | 200 | 1,753,431 | 5.979 | |
| 1400 | Pass | 1.01 | Pass | 36.371 | **Pass** | 1372 | 200 | 1,485,436 | 7.059 | Largest DF-safe tested MTU |
| 1420 | Pass | 1.01 | Pass | 31.849 | Fail | 1392 | 200 | 1,925,769 | 5.445 | DF loss |
| 1450 | Pass | 1.01 | Pass | 36.406 | Fail | 1422 | 200 | 1,711,902 | 6.125 | DF loss |
| 1472 | Pass | 1.01 | Pass | 24.639 | Fail | 1444 | 200 | 2,165,884 | 4.841 | DF loss |
| 1500 | Pass | 1.02 | Pass | 30.960 | Fail | 1472 | 200 | 2,676,191 | 3.918 | DF loss |

## Planned test records

Copy this block for each completed run.

### `<platform> - <network> - <date>`

| Field | Value |
|---|---|
| Client version / commit | `<value>` |
| Server version / commit | `<value>` |
| Device and OS | `<value>` |
| Network | `<Wi-Fi / Ethernet / LTE / 5G>` |
| IP stack | `<IPv4 / IPv6 / dual-stack>` |
| MTU range and step | `<for example: 1280-1500, step 20>` |
| Largest DF-safe MTU | `<value>` |
| Candidate operational MTU | `<value>` |
| Test method | `<commands or link to script>` |
| Raw data | `<relative CSV path>` |
| Notes | `<observations>` |

| MTU | Connect | Ping | DF probe | HTTP/Curl | Notes |
|---:|---|---|---|---|---|
| `<mtu>` | `<Pass/Fail>` | `<Pass/Fail>` | `<Pass/Fail>` | `<Pass/Fail>` | `<details>` |

## Decision log

| Date | Decision | Rationale | Scope |
|---|---|---|---|
| 2026-08-31 | Use 1400 as the Linux baseline candidate | Largest tested MTU without DF probe loss | Linux client to tested MASQUE-server path |
