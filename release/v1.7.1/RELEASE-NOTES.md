v1.7.1 is GitHub **Latest**. It replaces [v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3).

**Upgrade the Linux server and the Android and Windows clients together.** `connect-ip-go` v0.4.0 puts a request id on ADDRESS_ASSIGN. A 1.5.x client will not get an address from a 1.7.1 server, and a 1.7.1 client will not get one from a 1.5.x server.

The current iOS TestFlight build (**v1.7.0**, Apple build 20) is still on the old capsule format. Leave that phone on a 1.5.x server, or wait for a new TestFlight build. This tag does not include an IPA.

## What changed

- Android shows Connected only after the QUIC socket is kept off the VPN (`protect` + bind to the physical network, before the TUN route) and an ICMP echo to the server tunnel address comes back. Two automatic retries if the echo fails.
- `connect-ip-go` 0.3.0 → 0.4.0. `quic-go` 0.63.0.
- Android Gradle Plugin 9.4.1 on the Gradle 9.8 wrapper.
- Android `1.7.1` (`versionCode` 23). Windows product **1.7.1**.

## Files

- `masque-1.7.1.msi` — Windows VPN client
- `masque-setup.exe` — experimental VPS installer
- `masque-phone-1.7.1.apk` — Android phone
- `masque-tv-1.7.1.apk` — Android TV
- `vpn-server-linux-amd64` / `vpn-server-linux-arm64`
- `install.sh`, `gen-config.sh`, `SHA256SUMS`
