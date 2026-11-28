# v2.2.0 — 28 November 2026

GitHub **Latest**. Client-only: Android phone APK, Android TV APK, Windows MSI, and the `masque-setup.exe` helper. The published **server stays [v2.1.0](https://github.com/Next1971/masque-vpn/releases/tag/v2.1.0)** — do not replace the VPS binary for this release.

**iOS 2.2.0 ships through [TestFlight](https://testflight.apple.com/join/x52N41V1) by 7 December 2026.** The published TestFlight build stays 2.1.1 until then. No IPA in this drop.

## Multiple profiles on every platform

Store several imported profiles and pick the active one in the app:

- **Android phone:** profile spinner with a Delete button.
- **Android TV:** a compact picker row one D-pad press above Connect; Connect stays the first control.
- **Windows:** profile dropdown with a Delete button. The service answers `list_profiles`, `set_active`, and `delete_profile` over the named pipe.
- **iOS (TestFlight):** profile menu with Delete; the Packet Tunnel extension needs no update and reads the active profile.

Each profile lives in its own directory (certificates plus metadata), and one active-profile pointer selects what Connect uses. The profile id combines the server name with the client certificate hash, so re-importing a bundle updates that entry instead of creating a duplicate. Pre-2.2 single-profile installs migrate automatically on first use. Switching profiles while connected disconnects the current session first, after a confirmation. Transport, MTU, kill switch, autoconnect, and traffic counters stay global per device.

## masque-setup.exe

Attached to this release again. Its embedded install script defaults to the **v2.1.0** server (an earlier copy fell back to v2.0.0 when no Linux binary sat beside the EXE; the v2.1.0 release asset was replaced accordingly in November 2026), and the wizard fits smaller 14-inch displays.

## Dependency updates since v2.1.3

- connect-ip-go 0.7.0 with quic-go v0.65.0 (Windows and Android Go modules).
- golang.org/x/net 0.63.0 (Windows and Android), golang.org/x/crypto 0.59.0 (Windows), golang.org/x/sys 0.50.0 (Android).
- Android Gradle plugin 9.5.2 and Gradle wrapper 9.9.1.

## Compatibility

- Profiles and certificates do not change. Keep the v2.1.0 server.
- Install the v2.2.0 APK or MSI over the previous client. A v1.7.1 client still reaches the v2.1.0 server over UDP. TCP requires a v2.0.0 or v2.1.0 server.
- Android `2.2.0` / `2.2.0-tv` (`versionCode` 30). Windows product **2.2.0**.

## Files

- `masque-phone-2.2.0.apk` — Android phone
- `masque-tv-2.2.0.apk` — Android TV
- `masque-2.2.0.msi` — Windows VPN client
- `masque-setup.exe` — Windows VPS installer (deploys the v2.1.0 server)
- `SHA256SUMS`
