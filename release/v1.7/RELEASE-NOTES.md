v1.7 is **iOS TestFlight only**. It does **not** replace **Latest**. Daily driver stays **[v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3)**. Optional IPv6 QUIC remains **[v1.5.6](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.6)**.

**No APK, MSI, or Linux server binary is attached.** Install the iOS client from TestFlight.

## TestFlight

- Public link: [https://testflight.apple.com/join/x52N41V1](https://testflight.apple.com/join/x52N41V1)
- Marketing version **1.7.0**, Apple build **20** (external testing approved).
- Built from GitHub Actions [TestFlight run 20](https://github.com/Next1971/masque-vpn/actions/runs/33980112341) on `e708ea7` (`fix/ios-reconnect-hold`).

## Who should use this

Anyone with an iPhone or iPad who already runs a MASQUE server. Import the same `profile.masque` as Android/Windows. **One bundle per device.**

## Compatibility

This TestFlight build is the iOS client as of 5 September 2026. It does **not** include later `main` work (optional `alt_port`, QUIC over IPv6). Use an IPv4 profile and a single UDP port, same as Latest **v1.5.3**.

## Files

None on GitHub. The IPA is distributed by Apple TestFlight only.
