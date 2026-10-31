# MASQUE VPN — iOS client (TestFlight)

The current published iOS client is iOS 2.0, available through [public TestFlight](https://testflight.apple.com/join/x52N41V1). It uses the private CA and connects to the v2.1.0 server over UDP only. TCP does not carry traffic on iOS yet. No IPA is attached to GitHub Releases. This is not an App Store availability announcement.

iOS 1.7 / Apple build 20 was UDP-only as well. Its 4 December 2026 expiry is historical and must not be used as the current iOS 2.0 expiry. Check the installed build number and actual expiration in TestFlight; this guide does not assert an unverified current build number or date.

Swift provides the host app and Packet Tunnel Network Extension; gomobile builds Mobile.xcframework from the shared Go core. The simulator can compile UI/code but cannot validate the real VPN path; use a physical device. The project targets iPhone; check current device availability in TestFlight.

## Use

1. Install Apple's TestFlight and open the [public join link](https://testflight.apple.com/join/x52N41V1).
2. Install the current iOS 2.0 build.
3. Import a real profile.masque issued for this device; the sample is format-only. Do not reuse an active profile from another device.
4. Allow VPN configuration when iOS asks, then connect.
5. Verify the VPS exit address, DNS, and application traffic.

Existing profiles and certificates remain unchanged. iOS connects over UDP only. Older servers can still be used over UDP.

Ping on iOS is the smoothed QUIC RTT.

## Layout and identifiers

| Path | Purpose |
|---|---|
| Masque/ | Host app and VPN controls |
| PacketTunnel/ | NEPacketTunnelProvider and Go bridge |
| Shared/ | App Group and profile parsing |
| Frameworks/ | Locally built Mobile.xcframework |
| scripts/build-xcframework.sh | Go framework build |

| Target | Identifier |
|---|---|
| App | com.next1971.masque |
| Packet Tunnel | com.next1971.masque.packet-tunnel |
| App Group | group.com.next1971.masque |

The Go core runs inside the extension, not the host app. The App Group makes the profile available to the extension. Underlying transport routing must avoid looping through the VPN.

## Apple Developer setup

Use a paid Apple Developer team for signed device/TestFlight distribution. Register both App IDs with Network Extensions / Packet Tunnel and App Groups, assign the shared App Group to both, create the app in App Store Connect, and set the signing team on both Xcode targets.

## Build on a Mac

Use the checked-out project's Xcode requirements and Go 1.25.5 or later. The existing project documentation uses Xcode 16+ as a baseline; the selected CI runner/workflow is authoritative for its build environment. A main-branch build is not automatically the published TestFlight binary.

From the repository root:

```bash
chmod +x ios/scripts/build-xcframework.sh
./ios/scripts/build-xcframework.sh
open ios/Masque.xcodeproj
```

Select the team on both targets, then archive. For an unsigned simulator compile after building the framework:

```bash
cd ios
xcodebuild -project Masque.xcodeproj -scheme Masque \
  -destination 'generic/platform=iOS Simulator' \
  -configuration Debug CODE_SIGNING_ALLOWED=NO build
```

If generated Go symbols differ from Swift call sites, inspect the actual generated Mobile interface and rebuild the framework before changing bridge calls.

## TestFlight through GitHub Actions

The [TestFlight workflow](../.github/workflows/ios-testflight.yml) builds on a GitHub macOS runner, signs for App Store distribution, and uploads to TestFlight. Apple processing and external beta approval are separate from workflow success. Submitting an App Store listing for review is not the TestFlight upload step.

Keep signing material under the ignored ios/signing-local/ directory or another secure local location, never in Git.

### 1. API key and team

Create an App Store Connect API key with appropriate upload permissions. Save the .p8 privately and record Key ID, Issuer ID, and the Apple Team ID.

### 2. Distribution certificate

From the repository root (Git Bash/OpenSSL can be used on Windows):

```bash
mkdir -p ios/signing-local
cd ios/signing-local
openssl genrsa -out dist.key 2048
openssl req -new -key dist.key -out dist.csr -subj "/CN=MASQUE Apple Distribution/C=US"
```

Alternatively use scripts/make-distribution-csr.sh from ios/. Upload the CSR when creating an Apple Distribution certificate, download distribution.cer into the same directory, then run there:

```bash
openssl x509 -in distribution.cer -inform DER -out dist.pem
openssl pkcs12 -export -out dist.p12 -inkey dist.key -in dist.pem
```

Keep the export password for P12_PASSWORD. If your OpenSSL/import combination requires legacy PKCS#12 compatibility, inspect the workflow error before recreating the export.

### 3. Provisioning profiles

Create App Store Connect distribution profiles for both App IDs using that certificate. Save them as app.mobileprovision and tunnel.mobileprovision in ios/signing-local/. Both must carry the required capabilities and matching team.

### 4. Repository secrets

From the repository root:

```powershell
powershell -File ios/scripts/encode-secrets.ps1
```

Configure Actions secrets to match the workflow:

| Secret | Value |
|---|---|
| APP_STORE_CONNECT_KEY_ID | API Key ID |
| APP_STORE_CONNECT_ISSUER_ID | API Issuer ID |
| APP_STORE_CONNECT_API_KEY | Complete private .p8 PEM |
| APPLE_TEAM_ID | Developer Team ID |
| P12_PASSWORD | Certificate export password |
| BUILD_CERTIFICATE_BASE64 | Encoded dist.p12 |
| BUILD_PROVISION_PROFILE_APP_BASE64 | Encoded app profile |
| BUILD_PROVISION_PROFILE_EXT_BASE64 | Encoded extension profile |

Run Actions → TestFlight → Run workflow on the intended branch. Check signing/upload logs, then App Store Connect → TestFlight for processing and distribution status. Record the actual published build and expiry there. Never attach secret values to an issue.

## Security and limitations

- profile.masque includes a client private key. Import and transfer it privately.
- Keep insecure = false. A private-CA browser warning on the server's TCP page is not a reason to disable VPN server verification.
- The TCP page does not grant VPN access. A valid certificate and CONNECT-IP request are still required.
- IPv4/IPv6 transport and IPv4/IPv6 inside the tunnel are separate; validate the actual device path rather than assuming platform parity.
- The extension's APPLICATION_EXTENSION_API_ONLY setting and Go runtime integration require ongoing review; successful compilation is not an independent security audit.

See [server guide](../server/README.md), [client profiles](../docs/CLIENTS.md), [security policy](../SECURITY.md), and [roadmap](../docs/ROADMAP.md).
