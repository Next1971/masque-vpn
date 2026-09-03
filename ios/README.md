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

## TestFlight via GitHub (no Mac)

The workflow [.github/workflows/ios-testflight.yml](../.github/workflows/ios-testflight.yml) runs on GitHub’s Mac, signs an App Store IPA, and uploads it to **internal TestFlight**. You still need a physical iPhone to install it. Do **not** click Submit for Review in App Store Connect.

Do this once. Keep `dist.key` / `.p12` / `.p8` off git (they are gitignored).

### 1. API key (browser)

1. [App Store Connect](https://appstoreconnect.apple.com) → **Users and Access** → **Integrations** → **App Store Connect API**.
2. **Generate API Key**. Name `github-testflight`. Access **Admin** or **App Manager**.
3. Download the `.p8` (once). Copy **Key ID** and **Issuer ID**.

### 2. Team ID (browser)

[developer.apple.com/account](https://developer.apple.com/account) → Membership details → **Team ID** (10 characters).

### 3. Distribution certificate (Windows is fine)

Git for Windows includes `openssl` (Git Bash).

```bash
mkdir -p ios/signing-local
cd ios/signing-local
openssl genrsa -out dist.key 2048
openssl req -new -key dist.key -out dist.csr -subj "/CN=MASQUE Apple Distribution/C=US"
```

Or run `ios/scripts/make-distribution-csr.sh`.

1. [Certificates](https://developer.apple.com/account/resources/certificates/list) → **+** → **Apple Distribution** → upload `dist.csr` → download the `.cer`.
2. Put the `.cer` in `ios/signing-local` as `distribution.cer`, then:

```bash
cd ios/signing-local
openssl x509 -in distribution.cer -inform DER -out dist.pem
openssl pkcs12 -export -out dist.p12 -inkey dist.key -in dist.pem
```

If OpenSSL 3 errors on export, add `-legacy`. Choose a password; you will store it as `P12_PASSWORD`.

### 4. Two App Store profiles (browser)

[Profiles](https://developer.apple.com/account/resources/profiles/list) → **+** → **App Store Connect** (distribution), for each App ID, select the certificate from step 3:

| Download and rename to | App ID |
|---|---|
| `app.mobileprovision` | `com.next1971.masque` |
| `tunnel.mobileprovision` | `com.next1971.masque.packet-tunnel` |

Put both files in `ios/signing-local`.

### 5. GitHub Secrets

Repo → **Settings** → **Secrets and variables** → **Actions** → **New repository secret**.

From the `.p8` file, paste the **entire** PEM (including `BEGIN` / `END` lines) into `APP_STORE_CONNECT_API_KEY`.

Encode the three binaries (PowerShell, from the repo root):

```powershell
powershell -File ios/scripts/encode-secrets.ps1
```

That writes one-line `.txt` files in `ios/signing-local`. Paste each line into the matching secret:

| Secret | Value |
|---|---|
| `APP_STORE_CONNECT_KEY_ID` | Key ID from step 1 |
| `APP_STORE_CONNECT_ISSUER_ID` | Issuer ID from step 1 |
| `APP_STORE_CONNECT_API_KEY` | Full `.p8` text |
| `APPLE_TEAM_ID` | Team ID from step 2 |
| `P12_PASSWORD` | Password from the `pkcs12 -export` step |
| `BUILD_CERTIFICATE_BASE64` | contents of `BUILD_CERTIFICATE_BASE64.txt` |
| `BUILD_PROVISION_PROFILE_APP_BASE64` | `BUILD_PROVISION_PROFILE_APP_BASE64.txt` |
| `BUILD_PROVISION_PROFILE_EXT_BASE64` | `BUILD_PROVISION_PROFILE_EXT_BASE64.txt` |

### 6. Run the workflow

GitHub → **Actions** → **TestFlight** → **Run workflow** → branch `feature/ios-client`.

The first run often takes 20–40 minutes (gomobile). If it fails, open the log: missing secret, wrong profile type (must be App Store, not Development), or OpenSSL/p12 password.

When it succeeds: App Store Connect → **TestFlight** (not the store listing). After Apple processes the build (sometimes 10+ minutes), add yourself under Internal Testing and install **TestFlight** on an iPhone.

## Use

1. Install from TestFlight.
2. Import a real `profile.masque` (the sample in the app bundle is format-only).
3. Allow VPN when iOS asks.
4. Connect. Ping is QUIC RTT to the MASQUE server.

## Notes

- Go lives in the **extension**, not the host app. The profile is stored in the App Group so the extension can read certificates.
- There is no Android-style `protect(fd)`. The tunnel excludes the server IPv4 from the default route so QUIC does not loop into itself.
- `APPLICATION_EXTENSION_API_ONLY` is off for the extension because the Go runtime is not app-extension-safe. This is the usual gomobile VPN compromise.
