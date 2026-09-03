# MASQUE VPN — iOS client (TestFlight)

Minimal iPhone/iPad client on the same Go core (`clientcore`) as Android and Windows. Swift supplies the UI and a **Packet Tunnel** Network Extension; gomobile produces `Mobile.xcframework`.

This is the first iOS drop (version **1.6.0**). It has not been run on a device yet. The simulator cannot exercise the VPN path.

## Layout

```
ios/
├─ Masque.xcodeproj/
├─ Masque/                 # host app (import profile, start/stop VPN)
├─ PacketTunnel/           # NEPacketTunnelProvider + Go bridge
├─ Shared/                 # App Group + .masque parser
├─ Frameworks/             # Mobile.xcframework (built locally, not in git)
├─ scripts/build-xcframework.sh
└─ README.md
```

Bundle IDs:

| Target | ID |
|---|---|
| App | `com.next1971.masque` |
| Packet Tunnel | `com.next1971.masque.packet-tunnel` |
| App Group | `group.com.next1971.masque` |

Use the same `profile.masque` as Android/Windows. **One bundle per device** — do not import a profile that is already connected on another OS.

## Apple Developer (once)

1. Identifiers → App IDs: `com.next1971.masque` with **Network Extensions** (Packet Tunnel) and **App Groups**.
2. App ID `com.next1971.masque.packet-tunnel` with the same capabilities.
3. App Group `group.com.next1971.masque` on both IDs.
4. App Store Connect → new iOS app with bundle ID `com.next1971.masque`.
5. In Xcode: select your **Team** on both targets (Signing & Capabilities). Xcode should pick up the entitlements already in the repo.

## Build on a Mac

Requires **Xcode 16+**, **Go 1.25.5+** (CI uses 1.26.1), and a paid Apple Developer team for a device/TestFlight.

```bash
# 1. Shared Go core → xcframework (device + simulator)
chmod +x ios/scripts/build-xcframework.sh
./ios/scripts/build-xcframework.sh

# 2. Open the project, set the development team, then archive
open ios/Masque.xcodeproj
```

Or from the CLI (unsigned simulator compile, after the xcframework exists):

```bash
cd ios
xcodebuild -project Masque.xcodeproj -scheme Masque \
  -destination 'generic/platform=iOS Simulator' \
  -configuration Debug CODE_SIGNING_ALLOWED=NO build
```

If Swift cannot see `MobileDial` / `MobileConfig`, gomobile used un-prefixed names (`Dial`, `Config`) in the `Mobile` module — rename the calls in `PacketTunnelProvider.swift`.

## TestFlight

GitHub Actions on this branch compiles the xcframework and the app **unsigned**. Upload still needs secrets (not wired yet):

- App Store Connect API key (`.p8`, Key ID, Issuer ID)
- Signing certificates / profiles (or Fastlane Match)

Internal TestFlight: add testers in App Store Connect, then install TestFlight on a **physical iPhone**. Packet Tunnel does not run in the simulator.

## Use

1. Install from TestFlight.
2. Import a real `profile.masque` (the sample in the app bundle is format-only).
3. Allow VPN when iOS asks.
4. Connect. Ping is QUIC RTT to the MASQUE server.

## Notes

- Go lives in the **extension**, not the host app. The profile is stored in the App Group so the extension can read certificates.
- There is no Android-style `protect(fd)`. The tunnel excludes the server IPv4 from the default route so QUIC does not loop into itself.
- `APPLICATION_EXTENSION_API_ONLY` is off for the extension because the Go runtime is not app-extension-safe. This is the usual gomobile VPN compromise.
