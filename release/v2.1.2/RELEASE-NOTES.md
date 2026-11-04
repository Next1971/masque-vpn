v2.1.2 is an optional Android TV release. It does **not** replace Latest. The daily driver stays **[v2.1.0](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0)**.

Install this on Android TV instead of the v2.1.0 or v2.1.1 TV APK. Same profile, same server. `versionName` **2.1.2-tv**, `versionCode` **28**. It installs over the existing TV app (`com.next1971.masque.tv`).

The v2.1.1 screen fix is included: Connect is the first control and stays on a 540dp TV screen. This build also keeps the tunnel alive. The service holds a partial wake lock and a Wi-Fi lock while the VPN is up, so a sleeping TV still sends QUIC keepalives. If no QUIC packet arrives for 45 seconds, or TCP PINGs fail, the client drops the session and dials again. A finished TCP handshake is no longer reset by the dial timeout.

**No phone APK, Windows MSI, server binary, installer, or iOS build is attached.**

iOS on TestFlight stays **UDP only**. TCP does not carry traffic on iOS yet.

## Files

- `masque-tv-2.1.2.apk` — Android TV

SHA256: `839ca060f84b999c7309017ded0d38f02b0c8e0aabd8283036060846cbd37431`
