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
| Done | 2026-08-31 | Android | Android device | Wi-Fi | Not recorded | MASQUE server | 1280-1500 | **1400** | 1400 | Preliminary pass | DF probe passes through 1400; 100% loss at 1420 and above |
| Done | 2026-08-31 | Android | Android device | Mobile data | Not recorded | MASQUE server | 1280-1500 | **1370** | 1370 | Preliminary pass | DF probe passes through 1370; 100% loss at 1400 and above |
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

## Android client to MASQUE server

### Android over Wi-Fi — 2026-08-31

| MTU | Baseline ping loss | Baseline RTT avg, ms | DF probe loss | DF probe RTT avg, ms | Result | Notes |
|---:|---:|---:|---:|---:|---|---|
| 1280 | 0% | 66.1 | 0% | 139.4 | Pass | |
| 1300 | 0% | 115.1 | 0% | 78.4 | Pass | |
| 1350 | 0% | 138.4 | 0% | 94.3 | Pass | |
| 1400 | 0% | 135.1 | 0% | 110.6 | **Pass** | Largest DF-safe tested MTU |
| 1420 | 0% | 99.1 | 100% | - | Fail | DF probe lost |
| 1450 | 0% | 91.9 | 100% | - | Fail | DF probe lost |
| 1472 | 0% | 81.8 | 100% | - | Fail | DF probe lost |
| 1500 | 0% | 67.1 | 100% | - | Fail | DF probe lost |

**Preliminary result:** MTU **1400** is the largest tested value for which the Android Wi-Fi DF probe succeeds.

### Android over mobile data — 2026-08-31

| MTU | Baseline ping loss | Baseline RTT avg, ms | DF probe loss | DF probe RTT avg, ms | Result | Notes |
|---:|---:|---:|---|---:|---|---|
| 1280 | 0% | 131.0 | 0% | 140.7 | Pass | |
| 1300 | 0% | 128.6 | 0% | 118.3 | Pass | |
| 1350 | 0% | 118.0 | 0% | 148.5 | Pass | Source screenshot labels this row `1350 m0b`; treated as MTU 1350 |
| 1370 | 0% | 127.1 | 0% | 118.2 | **Pass** | Largest DF-safe tested MTU |
| 1400 | 0% | 99.7 | 100% | - | Fail | DF probe lost |
| 1420 | 0% | 129.8 | 100% | - | Fail | DF probe lost |
| 1450 | 0% | 128.1 | 100% | - | Fail | DF probe lost |
| 1472 | 0% | 119.5 | 100% | - | Fail | DF probe lost |
| 1500 | 0% | 121.0 | 100% | - | Fail | DF probe lost |

**Preliminary result:** MTU **1370** is the largest tested value for which the Android mobile-data DF probe succeeds. The next tested value, MTU 1400, fails; an additional narrow sweep between 1370 and 1400 is recommended before choosing a mobile-specific maximum.

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
| 2026-08-31 | Record Android Wi-Fi candidate MTU as 1400 | Largest tested MTU without DF probe loss; 1420 fails | Android client over tested Wi-Fi path |
| 2026-08-31 | Record Android mobile-data candidate MTU as 1370 | Largest tested MTU without DF probe loss; 1400 fails | Android client over tested mobile-data path |
