v2.1.3 is GitHub **Latest**. It is a **client-only** drop. The published server stays **[v2.1.0](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0)**. Same profiles and certificates.

Install the new phone APK, TV APK, or Windows MSI over the previous client. Do not replace the VPS binary, `install.sh`, Docker image, or `masque-setup.exe` for this release.

iOS stays on [TestFlight 2.1.1](https://testflight.apple.com/join/x52N41V1). No IPA is attached.

## What changed

- **Android phone:** Connect no longer freezes the app on filtered mobile data (whitelist). Dial runs off the UI thread, so Connect and Disconnect still respond while the handshake is in flight. `versionName` **2.1.3**, `versionCode` **29**.
- **Android TV:** Includes the v2.1.2 tunnel keep-alive (wake lock, silent-path redial) and the v2.1.1 Connect layout, plus the same Dial/Disconnect fix as the phone. `versionName` **2.1.3-tv**, `versionCode` **29**.
- **Windows:** After sleep the service rebuilds the tunnel instead of waiting for a manual Connect. A newer MSI replaces the running service on double-click. The window shows **v2.1.3**.

## Files

- `masque-phone-2.1.3.apk` — Android phone  
  SHA256: `9845ae739ba678ebcbf48a8265a358706d22f376f58259724b293234ddd7e8da`
- `masque-tv-2.1.3.apk` — Android TV  
  SHA256: `ed9eabb049f2a89b2a75a78416af9fe007f5a0be4f05c0506b9bc7964ce8bebe`
- `masque-2.1.3.msi` — Windows VPN client  
  SHA256: `f2c8e6dba2012ea9e4eb70069b18dfad09b90b4f8ad3c20c431131bc9192680f`

## Server

Keep the v2.1.0 server. Do not attach or install a new Linux binary from this tag.
