# MASQUE VPN — Android and Android TV

Android clients use Kotlin, VpnService, and a shared Go core packaged as masque.aar through gomobile. The published **v2.2.0** clients support HTTP/3 CONNECT-IP over QUIC/UDP and HTTP/2 CONNECT-IP over TLS/TCP. The server stays **v2.1.0**.

## What changed in v2.2.0

- **Phone and TV:** multiple profiles. Each imported profile lives in its own directory under `files/profiles/`, one active-profile pointer selects what Connect uses, and the profile id combines the server name with the client certificate hash — re-importing a bundle updates that entry instead of duplicating it. A pre-2.2 single-profile install migrates automatically. Transport, MTU, kill switch, and traffic counters stay global. `versionName` 2.2.0 / 2.2.0-tv, `versionCode` 30.
- **Phone:** a profile spinner with a Delete button; switching profiles while connected disconnects the current session after a confirmation.
- **TV:** the same storage with a compact picker row one D-pad press above Connect; Connect stays the first control, and Delete is disabled while the service owns a session.
- Dependency updates since v2.1.3: connect-ip-go 0.7.0 with quic-go v0.65.0, golang.org/x/net 0.63.0, golang.org/x/sys 0.50.0, Android Gradle plugin 9.5.2, Gradle wrapper 9.9.1.

## Install a published APK

1. Download masque-phone-2.2.0.apk and masque-tv-2.2.0.apk from [v2.2.0](../../releases/tag/v2.2.0). Install over the previous flavor; phone and TV packages stay distinct.
2. Allow installation from the source used to open the APK, then install the correct flavor.
3. Import a real profile.masque issued for this device. The bundled sample is a format example, not working credentials.
4. Grant VPN permission. On phones, allow the battery-optimization exemption if requested.
5. Choose Automatic, UDP only, or TCP only; leave MTU at the default 1340 initially. Connect.
6. Check that the public exit address is your VPS address. Disconnect using the app.

The kill switch is optional and off by default. Day/month traffic statistics and ping during TCP sessions are unchanged. QUIC ping is the smoothed QUIC RTT; TCP ping is not QUIC RTT. Automatic is not a fastest-transport benchmark.

Use one profile per device. Existing profiles and certificates remain unchanged. Keep the v2.1.0 server. Older servers remain usable over UDP; TCP requires a v2.0.0 or v2.1.0 server.

### Phone

Transfer profile.masque privately, open Import Profile, select it, and connect.

### Android TV

Use Paste config from clipboard while the app is in the foreground, or Import profile (paste text). Transfer the complete TV-specific profile privately; do not reuse the phone's active profile. Clipboard contents include credentials, so clear them after import where possible.

## Source tree and build versions

| Path | Purpose |
|---|---|
| app/src/main/ | Shared service, profile storage, and lifecycle |
| app/src/phone/ | Phone UI |
| app/src/tv/ | TV UI and paste import |
| app/libs/masque.aar | Locally built Go bridge |
| go-src/masque-vpn/ | Shared Go module and server |

Phone package: com.next1971.masque. TV package: com.next1971.masque.tv; both can be installed side by side.

These are the checked main-branch settings, not an assertion that every published APK was built with identical settings:

| Component | Setting |
|---|---|
| Go | 1.25.5 or later |
| JDK / JVM target | 17 |
| Android Gradle Plugin | 9.4.1 |
| Gradle wrapper | 9.8.1 |
| compileSdk / targetSdk / minSdk | 36 / 34 / 24 |
| NDK | 29.0.13599879 |

app/build.gradle.kts on this tree declares versionName 2.2.0 and versionCode 30 (TV suffix `-tv`). For an older published APK, inspect that release tag rather than assuming main matches the file you installed.

## Build from source

Install Android SDK Platform 36, JDK 17, and the pinned side-by-side NDK. Set ANDROID_HOME and ANDROID_NDK_HOME.

From android/ on Windows:

```bat
scripts\build-aar.bat
```

Manual AAR build from android/:

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
export PATH="$(go env GOPATH)/bin:$PATH"
gomobile init
cd go-src/masque-vpn
gomobile bind -target=android -androidapi 24 -o ../../app/libs/masque.aar ./mobile
```

Open android/ in Android Studio, select phone or tv, and build the APK. Outputs are under app/build/outputs/apk/. Rebuild the AAR whenever the Go core changes.

For a distributable release, configure keystore.properties with storeFile, storePassword, keyAlias, and keyPassword. Without a signing configuration the release output is unsigned; sign it before installation or distribution. Never commit the keystore or its passwords.

## How it works

VpnService establishes the VPN interface using the server-assigned address. The Go core forwards packets between that interface and CONNECT-IP. Platform integration keeps the underlying transport outside the VPN route. Android reconnect handling can retain the TUN while rebuilding a session after a network change.

Certificates are kept in app-private storage. A real profile contains the client private key.

## Limitations and troubleshooting

- One current server/profile; phone and TV use separate APKs.
- DNS inside the tunnel is plaintext UDP:53, visible at the VPS. This is not built-in DoH/DoT.
- The application kill switch depends on the service remaining alive. Use Android Always-on VPN / Block connections without VPN when OS-level blocking is required, and test it on the device.
- OEM battery restrictions can stop the service despite an exemption.
- Test UDP only and TCP only separately when diagnosing reachability. The VPS firewall and cloud security group must allow the corresponding protocol.
- A TCP website is not proof that VPN authentication or routing works. The private-CA browser warning is not a reason to disable client server-verification.
- IPv6 transport to the VPS and IPv6 packets inside the tunnel are different capabilities; verify both when needed.

See [server deployment](../server/README.md), [client profiles](../docs/CLIENTS.md), [security policy](../SECURITY.md), and [roadmap](../docs/ROADMAP.md). Earlier test results describe their tested builds and paths, not a blanket stability guarantee for every release.
