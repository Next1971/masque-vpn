v1.7.1 is a **client** drop: Android APK and Windows MSI. The Linux server already running (1.5.x, `connect-ip-go` v0.3.0) stays in place.

**Clients use `connect-ip-go` v0.4.0.** They only wait for the server's address assignment (`ReceiveAddressAssignment`) and do not send `ADDRESS_REQUEST`, so a 0.3 server still accepts the session. A 1.5.x client also still reaches that same server. This tag does not replace the server binary.

iOS TestFlight **v1.7.0** is unchanged. This tag does not include an IPA.

## What changed

- Android and Windows clients: `connect-ip-go` v0.3.0 → v0.4.0, still talking to a 0.3 server.
- Android shows Connected only after the QUIC socket is kept off the VPN (`protect` + bind to the physical network, before the TUN route) and an ICMP echo to the server tunnel address comes back. Two automatic retries if the echo fails.
- Android Gradle Plugin 9.4.1 on the Gradle 9.8 wrapper.
- `quic-go` 0.63.0 (already on main).
- Android `1.7.1` (`versionCode` 23). Windows product **1.7.1**.

## Files

- `masque-1.7.1.msi` — Windows VPN client
- `masque-phone-1.7.1.apk` — Android phone
- `masque-tv-1.7.1.apk` — Android TV
