# MASQUE VPN — Android client

A minimal Android VPN client on the same Go core (`clientcore`) as Windows/Linux: shared profile format and shared QUIC / HTTP/3 CONNECT-IP (MASQUE) logic.

The core is an `.aar` from **gomobile**. Kotlin supplies `VpnService`, a small UI, and profile import.

**v1.3** added QUIC keepalives, a battery-exemption prompt, reconnect without tearing the TUN, and Wi-Fi → LTE recovery (sticky `/32` on the server). **v1.3.1** added **Paste config from clipboard** on Android TV. **v1.4** added the app icon and on-screen **Ping**. **v1.4.1** bumps the Android toolchain (AGP 9 / Gradle 9), shows the **version** in the UI, sinks IPv6 so apps cannot bypass the VPN, and uses TUN `/24` on-link. **v1.5.0** forwards IPv6 through the tunnel when the server assigns a ULA address.

Use a **release APK** if you only want to connect. The rest of this file is for building from source.

---

## Contents

```
android/
├─ app/
│  ├─ src/main/             # shared: VpnService, ProfileStore, BatteryExemption
│  ├─ src/phone/            # handset UI (file import)
│  ├─ src/tv/               # leanback UI (clipboard + paste-text)
│  ├─ src/main/assets/sample-profile.masque   # format sample, not a real secret
│  └─ libs/                 # place masque.aar here (build step)
├─ go-src/masque-vpn/   # Go module (core + gomobile `mobile/` + server)
├─ scripts/build-aar.bat
└─ README.md
```

Product flavors (side by side):

| Flavor | Application ID | Import |
|---|---|---|
| **phone** | `com.next1971.masque` | **Import Profile** (file picker) |
| **tv** | `com.next1971.masque.tv` | **Paste config from clipboard** (preferred) or **Import profile (paste text)** |

Each device needs its own `profile.masque`. See [Issuing client configs](../docs/CLIENTS.md).

---

## Toolchain (must match the build)

These are the versions in `go.mod`, the Android project files, and GitHub Actions — not the older “Go 1.21 / SDK 34 only” wording.

| Piece | Version |
|---|---|
| **Go** | **1.25.5 or later** (`android/go-src/masque-vpn/go.mod`). CI uses **1.26.1**. |
| **JDK** | **17** (`sourceCompatibility` / `jvmTarget`; CI Temurin 17) |
| **Gradle** | **9.5.0** (wrapper; required by AGP 9.3+) |
| **Android Gradle Plugin** | **9.3.2** (built-in Kotlin; no separate `kotlin-android` plugin) |
| **compileSdk** | **36** |
| **targetSdk** | **34** (set explicitly; AGP 9 would otherwise default to compileSdk) |
| **minSdk** | **24** (Android 7.0; same as `gomobile bind -androidapi 24`) |
| **NDK** | **29.0.13599879** (r29 side-by-side), pinned as `ndkVersion` and in CI. Not the obsolete NDK package. |

In Android Studio, install via SDK Manager:

- Android SDK Platform **36** (compile) and platform tools
- **NDK (Side by side)** matching **29.0.13599879** (or set `ANDROID_NDK_HOME` to that folder)
- CMake if Studio offers it with the NDK

---

## Use a release APK

1. Download `masque-phone-1.5.0.apk` or `masque-tv-1.5.0.apk` from [v1.5.0](../../releases/tag/v1.5.0) ([Latest](../../releases/latest)).
2. Install the phone or TV APK (allow installation from unknown sources).
3. Import a real `profile.masque` from the server generator. The APK also ships a non-production `sample-profile.masque` in assets so you can see the expected format — do not use it to connect.
4. Grant **VPN** permission. On Connect the app may ask to **ignore battery optimizations** — allow it so keepalives can run with the screen off.
5. Connect. A MASQUE icon in the status bar means the VPN is up. The screen shows **Ping** (QUIC RTT to the server) and the app **version**.
6. Check `https://ifconfig.me` — you should see the **server** address.

Disconnect with **Disconnect** in the app.

### Phone

Transfer `profile.masque` to the device → **Import Profile** → pick the file → **Connect**.

### Android TV

Many TVs have no file manager and a broken IME paste. Prefer:

1. On a phone or PC, copy the **entire** contents of that TV’s `profile.masque`.
2. Get the text onto the TV clipboard (USB / nearby share / a TV browser — whatever works on that box).
3. In **MASQUE VPN (TV)** choose **Paste config from clipboard** (one remote click; the app must be in the foreground).
4. If the clipboard is empty or unreadable, use **Import profile (paste text)** and paste into the dialog.
5. **Connect** (VPN permission, then battery exemption if shown).

Do not reuse one bundle on the phone and the TV at the same time.

---

## Build from source

### 1. Build `masque.aar`

Set:

- `ANDROID_HOME` — SDK, e.g. `%LOCALAPPDATA%\Android\Sdk`
- `ANDROID_NDK_HOME` — NDK, e.g. `%LOCALAPPDATA%\Android\Sdk\ndk\29.0.13599879`

**Windows** (from `android\`):

```bat
scripts\build-aar.bat
```

**Manual / Linux** (same as CI):

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
export PATH="$(go env GOPATH)/bin:$PATH"
gomobile init

cd go-src/masque-vpn
gomobile bind -target=android -androidapi 24 -o ../../app/libs/masque.aar ./mobile
```

Success: `app/libs/masque.aar` (native `.so` for arm64 / armeabi-v7a / x86_64).

If `gomobile bind` complains about the NDK, check `ANDROID_NDK_HOME` points at **29.0.13599879**, not “NDK (obsolete)”.

### 2. APK in Android Studio

1. **File → Open** the `android/` directory. Accept `local.properties` / `sdk.dir` if asked.
2. Confirm `app/libs/masque.aar` exists (`implementation(files("libs/masque.aar"))`).
3. **Build → Build Bundle(s) / APK(s) → Build APK(s)** (uses the selected flavor), or **Generate Signed Bundle / APK** with your keystore.

Outputs land under `app/build/outputs/apk/` (e.g. `phone/debug/app-phone-debug.apk`, `tv/debug/app-tv-debug.apk`). Without `keystore.properties` a release APK is unsigned (sideload only). Rebuild `masque.aar` after Go-core changes (including ping) or the UI will not see them.

---

## How it works

- **VpnService** creates TUN with the **server-assigned** `/32` (two-phase: Dial → read address → `establish` → `StartWithFD`), `0.0.0.0/0`, and DNS from the profile. It tracks the underlying network (`NetworkCallback`, `setUnderlyingNetworks`, `protect` / `bindSocket` on the QUIC UDP fd) so Wi-Fi → LTE does not leave the socket on a dead path.
- The **Go core** wraps the TUN `fd` and forwards packets. If QUIC dies, the same TUN stays up and a new session is dialed. Keepalives: 15s period, 3 min idle.
- Certificates live in app-private `files/certs/`.
- After sleep, the UI stays on **Disconnect** if the VPN is still running (it does not show a false “profile ready / Connect”).

---

## Security

- A real `profile.masque` contains the **client private key**. Distribute out-of-band; do not commit it. The asset `sample-profile.masque` is a non-production format example.
- `.gitignore` excludes `app/libs/masque.aar`.

## Known limitations

- One server/profile (no list). UI is intentionally small.
- Tunnel DNS is plaintext UDP:53 (hidden from the local ISP, visible on the server).
- Some OEM battery savers ignore the exemption dialog; if the process is killed, Connect again.
- Phone and TV are separate APKs (build the **phone** or **tv** flavor in Studio).

Android stability tests for this line are complete (including **8 hours** of airplane mode, after which the tunnel came back cleanly). Other platform limits are listed in the [roadmap](../docs/ROADMAP.md).
