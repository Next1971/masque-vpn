# MASQUE VPN — Windows client

The published v2.1.0 Windows client supports HTTP/3 CONNECT-IP over QUIC/UDP and HTTP/2 CONNECT-IP over TLS/TCP, with private-CA client authentication and a Wintun adapter.

A LocalSystem service owns the tunnel; the Fyne GUI imports profiles and sends commands over a named pipe. Installation requires elevation, but normal GUI use does not. Closing the window leaves the tunnel running.

## Install from a release

1. Download masque-2.1.0.msi from [v2.1.0](../../releases/tag/v2.1.0).
2. Run it and approve the installation UAC prompt. The installation includes MasqueVpn, wintun.dll, masque-gui.exe, and the debug console client under C:\Program Files\MASQUE.
3. Open MASQUE VPN from the Start menu as a normal user.
4. Import a device-specific profile.masque. See [issuing client configurations](../docs/CLIENTS.md).
5. Select Automatic, UDP only, or TCP only. Start with MTU 1340, then connect and verify the VPS exit address.

The kill switch is off by default. v2.1.0 provides day/month traffic statistics and ping on TCP sessions. On QUIC, ping is smoothed QUIC RTT; TCP ping must not be described as QUIC RTT. Automatic is not a fastest-transport benchmark.

Use Disconnect to stop the tunnel; hiding/closing the GUI is not Disconnect. Profiles are stored under %ProgramData%\MASQUE\ rather than beside the EXE. Tray controls talk to the same service.

Prefer the single-file profile.masque format. The older TOML-plus-certs format remains documented for console debugging below; no unverified removal date is promised here.

### Compatibility

Existing profiles and certificates remain unchanged. A v2.1.0 client can use an older server over UDP. TCP requires a v2.0.0 or v2.1.0 server; the HTML response to unauthenticated TCP connections belongs to v2.1.0. Older UDP clients can still reach the current server over UDP.

## Install the server from Windows (masque-setup.exe)

Experimental: the helper connects as root and changes systemd, firewall rules, and credentials. Prefer the [manual server guide](../server/README.md) when you need to review each deployment step. Use a VPS you can recover or rebuild.

Download masque-setup.exe and the matching vpn-server-linux-amd64 or vpn-server-linux-arm64 from v2.1.0. Put them in the same folder, or select the binary in the UI. The installer does not embed the Linux server and is separate from the VPN-client MSI.

Supported targets: Ubuntu 22.04/24.04/26.04 or Debian 12, with systemd, apt, and /dev/net/tun.

1. Enter SSH host and root credentials, then connect and check the OS. Verify the host identity before trusting a new SSH key.
2. On a fresh VPS, choose the primary port and optional second port, then install. The v2.1.0 helper deploys UDP and TCP. Also allow both protocols in the cloud security group.
3. Save the generated device profile; the helper also displays a profile QR code. QR display alone does not establish scanner support on every client.
4. For an existing installation, preserve its CA and configuration. Use the issuance/revocation actions rather than reinstalling to add a device.
5. Issue next bundle saves masque-client-N.profile.masque. Bootstrap profile.masque uses the original bootstrap identity. Keep all profiles private.
6. Revoke certificate adds the selected CN to /opt/masque/blocked_cns and restarts the service. This is a server denylist, not a PKI CRL; existing sessions are interrupted and certificate files remain on disk.

The helper reserves test identities #1–8 and issues numbered bundles from #9. Do not equate issuance numbering with a benchmarked concurrent-session limit. ca.key stays on the VPS; back it up securely. SSH host keys are recorded under %AppData%\MASQUE\setup_known_hosts (trust on first use).

A helper reachability result does not replace authenticated UDP/TCP tests from the target device. After connection, check the exit address and actual traffic.

## Source layout

| Directory | Purpose |
|---|---|
| cmd/vpn-service/ | Windows tunnel service |
| cmd/vpn-gui/ | Fyne GUI |
| cmd/vpn-setup/ | VPS installer |
| cmd/vpn-client/ | Console/debug client and service commands |
| internal/clientcore/ | Tunnel connectivity and forwarding |
| internal/engine/, ipc/, store/, winnet/ | Lifecycle, named pipe, profile storage, Wintun/routes/DNS |
| internal/vpssetup/ | SSH deployment helper |
| installer/ | WiX MSI packaging |
| scripts/ | Build and installation scripts |

## Build from source

Use Windows 10/11 x64, Go 1.25.5 or later, and MinGW gcc for the Fyne GUI/CGO build. The service and console can use CGO_ENABLED=0. MSI packaging needs WiX; use the version selected by the checked-out workflow.

Install the compiler through an MSYS2 MINGW64 shell if needed:

```bash
pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config
```

Put the MinGW bin directory on PATH. From windows/:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build.ps1
powershell -ExecutionPolicy Bypass -File scripts\build-msi.ps1
```

The client build produces dist/masque-svc.exe, masque-gui.exe, vpn-client.exe, and wintun.dll. Use the appropriate setup build/workflow for the separate VPS helper. A main-branch build is not automatically the published v2.1.0 binary; use the matching tag for release reproduction.

Without MSI, install the service from elevated PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install-service.ps1
```

Then run the GUI as a normal user.

## Console debugging

Standalone mode requires an elevated terminal and does not use the service:

```bat
cd dist
vpn-client.exe -profile profile.client.toml -full-route -timeout 0
```

Use profile.client.toml.example and its certs directory. Relative certificate paths are resolved from the working directory. -full-route enables the full tunnel; -timeout 0 runs until interruption. -insecure skips server verification and must not be used for normal operation.

Commands for an already installed service:

```bat
vpn-client.exe -svc-status
vpn-client.exe -svc-import path\to\profile.masque
vpn-client.exe -svc-connect
vpn-client.exe -svc-disconnect
```

The service owns Wintun, routes, DNS, and reconnect. It keeps the underlying server connection outside the VPN route. The GUI only controls/displays service state.

## Troubleshooting

| Symptom | Checks |
|---|---|
| Service unavailable | Install MSI, check MasqueVpn in services.msc; log is %ProgramData%\MASQUE\masque-svc.log |
| wintun.dll missing | It must be beside masque-svc.exe; repair the installation |
| Access denied in standalone mode | Elevate the console; normal GUI use does not require elevation |
| UDP fails but TCP works | Compare selected mode, firewall protocols, port and network path |
| Certificate error | Check CA, profile, server_name/SAN, certificate dates and clock; keep insecure = false |
| TCP page but no tunnel | Page access is not VPN authentication; verify the device certificate and blocked CN list |
| Connected but no traffic | Routes, forwarding/NAT, DNS, MTU, and one unique bundle per device |
| GUI build cannot find gcc | Check MinGW installation and PATH |

Disconnect session closure was fixed in v1.5.6; do not use older release workarounds as the current installation instructions.

## Security and limitations

- Real profiles and client keys are secrets. Protect %ProgramData%\MASQUE\ and never commit credentials.
- insecure / -insecure removes the client's authentication of the server. A browser warning on the private-CA TCP page is not a reason to enable it.
- The optional kill switch depends on the service lifecycle. Stopping/killing the service is not guaranteed to preserve blocking; test the required failure cases.
- In-tunnel DNS is plaintext UDP:53 and is visible at the VPS.
- One current server/profile per machine. IPv6 reachability and IPv6 inside the tunnel must be verified separately.
- Experimental software; no independent security audit or guaranteed filtering bypass.

See [server guide](../server/README.md), [security policy](../SECURITY.md), and [roadmap](../docs/ROADMAP.md).
