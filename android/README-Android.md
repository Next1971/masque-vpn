# MASQUE VPN — Android client (build in Android Studio)

A minimal Android VPN client based on the same Go core (`clientcore`) as the
Windows/Linux versions. It uses a shared profile format and shared QUIC/CONNECT-IP (MASQUE) logic.

The core is integrated through **gomobile** (Go → `.aar`), with a thin 
Kotlin layer: `VpnService`, a minimal UI, and profile import.

---

## Contents

```
masque-android/
├─ app/
│  ├─ src/main/
│  │  ├─ java/com/next1971/masque/
│  │  │  ├─ MainActivity.kt        UI: profile import, connection button, status
│  │  │  ├─ MasqueVpnService.kt    VpnService: creates TUN and passes its fd to the Go core
│  │  │  └─ ProfileStore.kt        parses .masque files and writes certificates to storage
│  │  ├─ res/…                     layout + strings
│  │  └─ AndroidManifest.xml
│  ├─ libs/                        Place masque.aar HERE (step 2)
│  └─ build.gradle.kts
├─ go-src/masque-vpn-mvp/               Go core source + bridge (for gomobile bind)
│  ├─ mobile/masque.go             gomobile bridge (exports Connect/Tunnel/Config/Callback)
│  ├─ internal/clientcore/         shared core (the same as Windows/Linux)
│  ├─ cmd/…                        desktop wrappers (not required to build the AAR)
│  ├─ go.mod / go.sum
├─ scripts/build-aar.bat           builds masque.aar through gomobile (Windows)
├─ profile.masque                  ready-to-use profile with inline certificates
└─ README-Android.md               this file
```

---

## Prerequisites

1. **Android Studio** (latest stable version) with the following installed through SDK Manager:
   - Android SDK Platform 34
   - **NDK** (Side by side) — required by gomobile
   - CMake (usually installed with the NDK)
2. **Go** (already installed on Windows) — version 1.21 or later.
3. Internet access for the initial download of Gradle plugins and `gomobile`.

---

## Step 1. Build `masque.aar` (Go core)

gomobile converts the `mobile` Go package into an Android `.aar` library.

### Option A — use the script (recommended)

1. Set the environment variables (File Explorer → “Edit environment variables”):
   - `ANDROID_HOME` → SDK path, for example `C:\Users\YOUR_USER\AppData\Local\Android\Sdk`
   - `ANDROID_NDK_HOME` → NDK path, for example
     `C:\Users\YOUR_USER\AppData\Local\Android\Sdk\ndk\<version>`
2. Open `cmd` in the project directory and run:
   ```
   scripts\build-aar.bat
   ```
   The script installs `gomobile`/`gobind`, runs `gomobile init`, and builds
   `app\libs\masque.aar`.

### Option B — manual setup

```bat
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
set PATH=%GOPATH%\bin;%PATH%     REM  See GOPATH with `go env GOPATH`
gomobile init

cd go-src\masque-vpn-mvp
gomobile bind -target=android -androidapi 24 -o ..\..\app\libs\masque.aar .\mobile
```

On success, **`app/libs/masque.aar`** is created (several MB and contains
native `.so` libraries for arm64/arm/x86_64).

> If `gomobile bind` reports an NDK issue, check `ANDROID_NDK_HOME` and ensure the NDK
> is actually installed through SDK Manager (not only “NDK (obsolete)”).

---

## Step 2. Open and build the APK in Android Studio

1. Choose **File → Open** and select the `masque-android` directory.
2. Wait for Gradle Sync. On the first run, Studio may offer to create
   `local.properties` with `sdk.dir`; accept it (or it will be created automatically).
3. Ensure `app/libs/masque.aar` is present; otherwise the build will fail at
   `implementation(files("libs/masque.aar"))`).
4. **Build → Build Bundle(s) / APK(s) → Build APK(s)**.
5. The output file is under `app/build/outputs/apk/` (phone debug:
   `phone/debug/app-phone-debug.apk`).

A debug APK is sufficient for device installation. For a release signature, use
**Build → Generate Signed Bundle / APK** with your own keystore.

---

## Step 3. Install and run on a phone

1. Transfer `app-debug.apk` to the phone and install it (you must allow
   “installation from unknown sources”).
2. Transfer **`profile.masque`** to the phone’s device storage.
3. Open **MASQUE VPN** → **“Import Profile”** and select
   `profile.masque`. “Profile imported” should appear.
4. Tap **“Connect”**. Android will display the system VPN permission request;
   allow it. A key icon appears in the notification area (VPN active).
5. To test, open `https://ifconfig.me` in a browser; it should show
   `YOUR_SERVER_HOST` (the server IP), rather than your actual IP.

Disconnect using the **“Disconnect”** button in the app.

---

## How it works (briefly)

- **VpnService** (Kotlin) creates TUN through `Builder` using the **server-assigned** `/32` (two-phase: Dial → read address → `establish` → `StartWithFD`),
  route `0.0.0.0/0` (all traffic), and DNS from the profile. It obtains the interface
  file descriptor. It also tracks the current underlying network and asks Android
  to ignore battery optimization so QUIC keepalives can run with the screen off.
- The **Go core** receives this `fd`, wraps it in `tun.Device`
  (`CreateUnmonitoredTUNFromFD`), and forwards packets between TUN and the
  QUIC/CONNECT-IP tunnel. If QUIC dies, the same TUN is kept and a new session
  is dialed.
- **Android** (VpnService.Builder) configures routes, address, and DNS; the core
  does not modify them, keeping the bridge clean and portable.
- Certificates (mTLS) are stored in the app’s internal storage
  (`files/certs/`), which is inaccessible to other apps.

## Security

- `profile.masque` contains the **client private key**. Treat it as a secret;
  do not publish or commit it to the repository.
- `.gitignore` already excludes the `app/libs/masque.aar` binary from git.

## Known limitations (E stage)

- One server/profile (no list). The UI is intentionally minimal.
- IPv4 only in the tunnel (IPv4 server).
- Tunnel DNS uses plaintext UDP:53 (hidden from the local provider but visible on the
  server). DoH/DoT are planned for the future.
- Phone and TV are separate APKs (`:app:assemblePhoneDebug` / `:app:assembleTvDebug`).
  The TV app talks to the same server; rebuild TV only if you want the v1.3 client
  behaviour on the set-top box.
<!-- Build status refreshed -->
