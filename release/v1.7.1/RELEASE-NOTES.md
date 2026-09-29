v1.7.1 is GitHub **Latest**. It replaces [v1.5.3](https://github.com/Next1971/masque-vpn/releases/tag/v1.5.3).

**Same CONNECT-IP capsules as 1.5.x.** A 1.7.1 client reaches a 1.5.x server, and a 1.5.x client reaches a 1.7.1 server. `connect-ip-go` stays at v0.3.0.

iOS TestFlight **v1.7.0** is unchanged. This tag does not include an IPA.

## What changed

- Android shows Connected only after the QUIC socket is kept off the VPN (`protect` + bind to the physical network, before the TUN route) and an ICMP echo to the server tunnel address comes back. Two automatic retries if the echo fails.
- Android Gradle Plugin 9.4.1 on the Gradle 9.8 wrapper.
- `quic-go` 0.63.0 (already on main).
- Android `1.7.1` (`versionCode` 23). Windows product **1.7.1**.

## Files

- `masque-1.7.1.msi` — Windows VPN client
- `masque-setup.exe` — experimental VPS installer
- `masque-phone-1.7.1.apk` — Android phone
- `masque-tv-1.7.1.apk` — Android TV
- `vpn-server-linux-amd64` / `vpn-server-linux-arm64`
- `install.sh`, `gen-config.sh`, `SHA256SUMS`
