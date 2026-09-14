# MASQUE VPN — Windows client

A Windows VPN client on the shared Go core (`clientcore`). It speaks **QUIC + HTTP/3 CONNECT-IP (MASQUE, RFC 9484)** with mutual TLS, and tunnels traffic through a **Wintun** adapter.

From **v1.3.1** the normal install is a per-machine **MSI**: a **LocalSystem** service runs the tunnel, and a **Fyne GUI** (Start menu, no UAC) imports a profile and connects. Closing the window does not tear down the tunnel. The console `vpn-client.exe` remains for debug. **v1.4** adds the app icon (tray, window, Start menu, MSI, EXE) and **Ping** on the GUI (smoothed QUIC RTT to the server). **v1.5.0** adds IPv6 on the tunnel when the server assigns it (GUI and console). **v1.5.1** uses TUN MTU **1369** and adds **certificate revoke** in `masque-setup.exe`. **v1.5.2** adds an optional kill switch (default off). **v1.5.3** is GitHub **Latest** (same Windows client as 1.5.2, product version bump). **v1.5.4** (pre-release) bumps `connect-ip-go` and races optional `alt_port`. **v1.5.6** adds QUIC over IPv6 and makes **Disconnect** close the session immediately.

---

## Install from a release (recommended)

1. Download `masque-1.5.3.msi` from [v1.5.3](../../releases/tag/v1.5.3) ([Latest](../../releases/latest)). Optional: `masque-1.5.6.msi` from [v1.5.6](../../releases/tag/v1.5.6) if you want IPv6 QUIC or the Disconnect fix — not required if 1.5.3 already works.
2. Run the installer (one UAC prompt). It installs `MasqueVpn` (auto-start), `wintun.dll`, `masque-gui.exe`, and `vpn-client.exe` under `C:\Program Files\MASQUE`.
3. Open **MASQUE VPN** from the Start menu (no admin).
4. **Import profile**: `profile.masque` (same single-file bundle as Android).

   A second, older form still works: `profile.client.toml` together with its `certs/` folder (`ca.crt`, `client.crt`, `client.key`). It will be removed in the next release — use `profile.masque`.
5. Click **Connect**. While connected, the GUI shows **Ping** to the server. Optional: **Connect automatically when the service starts**.

Each device needs its own bundle. See [Issuing client configs](../docs/CLIENTS.md).

The imported profile is stored under `%ProgramData%\MASQUE\` (not next to the EXE). The GUI talks to the service over a named pipe; the tray **Show / Connect / Disconnect** items do the same. Closing or hiding the window leaves the tunnel up — use **Disconnect**. From **v1.5.6**, Disconnect closes the QUIC session right away (the adapter and routes come down with it). On **v1.5.4** and earlier the GUI could stay on Connected until a packet arrived; workaround there is Services → **MASQUE VPN** → Stop (`sc stop MasqueVpn`).

---

## Install the server from Windows (`masque-setup.exe`)

**Experimental.** This talks to the VPS as **root**, can change firewall and systemd, and can **revoke** a client CN (denylist, not a PKI CRL). Prefer the [manual server guide](../server/README.md) if you need a known-good install. Do not run it against a VPS you cannot rebuild.

This is a separate app from the VPN client (not in the MSI). It SSHes to a **root** VPS and runs the same layout as [server/README.md](../server/README.md).

**Put the Linux server binary next to the EXE:** `vpn-server-linux-amd64` or `vpn-server-linux-arm64` from the **same** [v1.5.6 release](../../releases/tag/v1.5.6) (or pick the file in the UI). The installer does not contain the server. The wizard still chooses **one** UDP port and a Public host that is **IPv4 or DNS** (no IPv6 literal). Dual-port and IPv6 dial stay in `gen-config.sh --alt-port` / `--dial`.

**Supported OS:** Ubuntu **22.04**, **24.04**, or **26.04**, or Debian **12**, with systemd, `apt`, and `/dev/net/tun`. Anything else is refused.

1. Download `masque-setup.exe` and the matching `vpn-server-linux-*` from [v1.5.6](../../releases/tag/v1.5.6). Keep them in one folder.
2. Enter SSH host, root password or key, and **Connect and check OS**. If MASQUE is **already installed**, the app **does not reinstall** (no new CA, no new `server.crt`). Port pick / Install are disabled; use **Issue next bundle**.
3. Pick a suggested UDP port (443, 2053, 8443, 41234 if not already listening) and **Confirm** — only when installing onto a blank VPS.
4. **Install**. If `ufw` exists, UDP is allowed there. **Reachability OK** means this PC got a QUIC reply **after** the service was listening. ICMP ping is not used. A timeout usually means the **cloud security group** still blocks UDP.
5. **Issue next bundle (#9+)** saves `masque-client-N.profile.masque` (import on Android or in the Windows GUI). **Save bootstrap profile.masque** is the unnumbered cert from install (`CN=masque-client`). `ca.key` stays at `/opt/masque/ca`.
6. **Revoke certificate**: enter the issued number (the `N` in `masque-client-N`), confirm, and the app appends that CN to `/opt/masque/blocked_cns` then restarts `masque.service`. The PEM files stay on disk; the server refuses that CN after restart.

**Test certificates #1–8.** Existing bundles whose client CN is `masque-client-1` … `masque-client-8` (the first test pool) are **left alone** on issue. The app never issues those numbers, does not replace `server.crt`, and does not create a new CA — so those devices keep working unless you revoke them. They do not consume the app counter. The counter is **app-issued from #9**, shown as `N/253` (the tunnel IP pool size). Numbers 1–8 are reserved whether or not those files still exist on the VPS.

SSH host keys are stored under `%AppData%\MASQUE\setup_known_hosts` (TOFU).

Verify the exit IP:

```bat
curl -4 http://ifconfig.me/ip
```

It should show the **server’s** IP, not your ISP’s.

### TLS troubleshooting

Server certificate checks are **on by default**. If a self-signed CA or hostname mismatch blocks the handshake, you can skip verification only while debugging:

- Console: `-insecure`
- Profile: `insecure = true` under `[tls]`

Do not leave this enabled for daily use.

---

## Repository layout

```
windows/
├─ cmd/
│  ├─ vpn-service/          # LocalSystem service → dist\masque-svc.exe
│  ├─ vpn-gui/              # Fyne GUI → dist\masque-gui.exe
│  ├─ vpn-setup/            # Fyne VPS installer → dist\masque-setup.exe
│  └─ vpn-client/           # console + -svc-* CLI → dist\vpn-client.exe
├─ internal/
│  ├─ clientcore/           # QUIC, mTLS, CONNECT-IP, forwarding
│  ├─ engine/               # service tunnel lifecycle
│  ├─ ipc/                  # named-pipe protocol (GUI ↔ service)
│  ├─ store/                # ProgramData profile + certs
│  ├─ vpssetup/             # SSH install helper used by masque-setup
│  └─ winnet/               # Wintun, routes, DNS
├─ installer/               # WiX per-machine MSI + masque.ico
├─ scripts/
│  ├─ build.bat / build.ps1
│  ├─ build-msi.ps1
│  ├─ install-service.ps1   # copy dist\ into Program Files without MSI
│  └─ fetch-wintun.ps1
├─ certs/README.md
├─ profile.client.toml.example
└─ README.md
```

---

## Prerequisites (build from source)

| Tool | Version |
|---|---|
| **Go** | **1.25.5 or later** (`windows/go.mod`). CI builds with **1.26.1**. |
| OS | Windows 10/11 **x64** |
| **MinGW gcc** | Required only for `masque-gui.exe` (Fyne + CGO). Service and console are `CGO_ENABLED=0`. |
| **WiX** | Optional; for `masque.msi` (`dotnet tool install --global wix`, v7 as in CI). |

Install MinGW if the GUI build cannot find `gcc`:

```bat
pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config
```

(from an **MSYS2 MINGW64** shell; `gcc` typically lives in `C:\msys64\mingw64\bin`).

Administrator rights are needed to **install** the service or MSI, not to **build**, and not to use the GUI after install.

---

## Build

From `windows\`:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build.ps1
```

or `scripts\build.bat`. Output in `dist\`:

- `masque-svc.exe`
- `masque-gui.exe`
- `vpn-client.exe`
- `wintun.dll` (fetched if missing)

MSI:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build-msi.ps1
```

Without an MSI, from an **elevated** PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install-service.ps1
```

That copies `dist\*` to `C:\Program Files\MASQUE` and creates/starts `MasqueVpn`. Then run `masque-gui.exe` as a normal user.

---

## Console debug (Administrator)

Standalone mode does **not** use the service. Run an elevated terminal:

```bat
cd dist
vpn-client.exe -profile profile.client.toml -full-route -timeout 0
```

Use a copy of `profile.client.toml.example` plus `certs\` next to the EXE (paths in the TOML are relative to the working directory).

Talk to an **already installed** service (no extra admin if the pipe is available):

```bat
vpn-client.exe -svc-status
vpn-client.exe -svc-import path\to\profile.masque
vpn-client.exe -svc-connect
vpn-client.exe -svc-disconnect
```

### Command-line flags

| Flag | Default | Meaning |
|---|---|---|
| `-profile` | (standalone req.) | Path to client TOML |
| `-full-route` | false | Route all traffic via the tunnel |
| `-timeout` | 25s | Overall timeout; `0` runs until Ctrl+C |
| `-insecure` | false | Skip server certificate verification |
| `-svc-status` / `-svc-connect` / `-svc-disconnect` | | Named-pipe commands to the service |
| `-svc-import` | | Import a profile file via the service |
| `-test` | true | Test mode: only `-test-dst` via TUN |
| `-test-dst` | 1.1.1.1 | Test-mode destination |
| `-ping` | 3 | Test-mode ICMP echo count |

For a real VPN in console mode you only need `-profile` and `-full-route` (with a long `-timeout`, e.g. `0` or `24h`). `-full-route` already installs a default route; `-test` only applies when full-route is off.

---

## How it works

- `internal/clientcore` dials QUIC, does mTLS, opens CONNECT-IP, and forwards packets. It does not edit the OS routing table.
- `masque-svc.exe` owns Wintun, routes (`0.0.0.0/0` plus a host route to the server via the original gateway so QUIC does not loop), DNS on `masque0`, and reconnect. Keepalives match the server (15s / 3 min idle, **v1.3**).
- `masque-gui.exe` sends import / connect / disconnect / autoconnect / kill switch over IPC and shows status, assigned IP, and ping.

---

## Troubleshooting

- **Service unavailable** in the GUI — install the MSI (or `install-service.ps1`) and confirm `MasqueVpn` is running in `services.msc`. Log: `%ProgramData%\MASQUE\masque-svc.log`.
- **Disconnect does nothing (pre-1.5.6)** — the session stayed up until a packet arrived. Stop the service (`sc stop MasqueVpn`) or install **v1.5.6**, which closes QUIC on Disconnect.
- **wintun.dll not found** — it must sit next to `masque-svc.exe` (the MSI does this).
- **Access is denied** in console mode — elevate the terminal; the GUI path does not need UAC after install.
- **No traffic** — check `server` / `server_name` vs the certificate SAN, and that the bundle matches this server’s CA. One bundle per device.
- **GUI build fails (gcc)** — put MinGW `gcc` on `PATH` as in Prerequisites.
- **`go` too old** — need **1.25.5+**, not 1.21.

---

## Security

- Never commit real `*.crt` / `*.key` or a filled-in profile. `.gitignore` already excludes them.
- Treat `%ProgramData%\MASQUE\certs\` as secrets.
- `insecure` / `-insecure` authenticates only the client to the server; the client no longer authenticates the server.

## Limitations

- In-tunnel DNS is plaintext UDP:53 (hidden from the local ISP, visible at the server).
- Kill switch (v1.5.2+, optional) blackholes traffic while the MASQUE service is running and Connect has been requested. Stopping the service or killing the process restores the normal default route.
- Single server/profile per machine.

Other platform limits are listed in the [roadmap](../docs/ROADMAP.md).
