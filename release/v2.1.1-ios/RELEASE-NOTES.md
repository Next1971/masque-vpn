v2.1.1 (iOS) is a TestFlight release. It does **not** replace Latest. The daily driver for Android phone, Windows, and the server stays **[v2.1.0](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0)**. Android TV stays on the optional **[v2.1.2](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.2)** APK. The tag [v2.1.1](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.1) remains the older Android TV build.

iOS has caught up with the other clients. The same profile works over UDP and over TCP. Traffic counters, ping, and the connect status are on the same screen as Android and Windows.

Marketing version **2.1.1**, Apple build **40**. Install from [TestFlight](https://testflight.apple.com/join/x52N41V1). Built by [TestFlight run 40](https://github.com/Next1971/masque-vpn/actions/runs/42070104510).

**No phone APK, Windows MSI, server binary, installer, or IPA is attached.** The IPA is distributed by Apple TestFlight only.

## Fixed

- UDP and TCP both carry traffic. TCP uses the packet-tunnel socket. A dial from inside the tunnel is no longer captured by the default route, and a finished handshake is no longer reset.
- Connect no longer freezes the app and then closes it. TCP writes return as soon as the bytes are queued, so the Go thread is not stuck waiting on the socket.
- The status line stays **VPN active**. Long status text no longer wraps and shifts the buttons.
- Today and Month traffic counters update while the tunnel is up.
- After sleep, a frozen session dials again instead of staying up with a dead QUIC connection.

## Files

None on GitHub. The IPA is distributed by Apple TestFlight only.
