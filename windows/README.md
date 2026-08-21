# MASQUE VPN — Windows Client

A minimal Windows VPN client built on top of a portable Go core (`clientcore`)
shared with the Android and Linux clients. It speaks **QUIC + HTTP/3 CONNECT-IP
(MASQUE, RFC 9484)** with mutual TLS to a MASQUE server, and tunnels all traffic
through a Wintun adapter. From **v1.3** the shared core enables QUIC keepalives
(15s / 3 min idle), matching the server.

This repository contains everything needed to **build `vpn-client.exe` from
source on Windows**.

\---

## Repository layout

```
masque-windows-client/
├─ cmd/vpn-client/            # platform wrapper (TUN + routing + DNS)
│  ├─ main.go                 # flags, profile loading, run loop
│  ├─ iface\\\_windows.go        # Windows: Wintun, routes, DNS (build tag)
│  └─ iface\\\_linux.go          # Linux equivalent (build tag)
├─ internal/clientcore/       # shared, platform-independent core
│  ├─ client.go               # QUIC dial, mTLS, CONNECT-IP session, forwarding
│  ├─ iphdr.go                # IPv4/IPv6 header helpers (TTL normalization)
│  └─ profile.go              # TOML profile parsing + validation
├─ dist/
│  └─ wintun.dll              # Wintun driver 0.14.1 (amd64), required at runtime
├─ scripts/
│  ├─ build.bat               # build via cmd.exe
│  └─ build.ps1               # build via PowerShell
├─ certs/                     # put ca.crt / client.crt / client.key here
│  └─ README.md
├─ profile.client.toml.example
├─ go.mod / go.sum
└─ README.md                  # this file
```

\---

## Prerequisites

* **Go 1.21+** for Windows — https://go.dev/dl/ (verify with `go version`)
* Windows 10/11 x64
* Administrator rights (only to **run** the VPN, not to build it)

No CGO, no C toolchain required — it is pure Go.

\---

## Build

### Option A — script

From `cmd.exe`:

```bat
scripts\\\\build.bat
```

or from PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\\\\build.ps1
```

### Option B — manual

```bat
go mod download
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o dist\\\\vpn-client.exe .\\\\cmd\\\\vpn-client
```

Output: **`dist\\\\vpn-client.exe`** (\~11 MB). `dist\\\\wintun.dll` is already
included and must stay next to the EXE.

\---

## Configure

1. Copy the profile template and edit it:

```bat
   copy profile.client.toml.example dist\\\\profile.client.toml
   ```

Set `server`, `server\\\_name`, and the DNS you want. `server\\\_name` must match
the server certificate's CN/SAN.

2. Put your mTLS certificates in `dist\\\\certs\\\\`:

   * `ca.crt`     — CA that signed the server certificate
   * `client.crt` — your client certificate
   * `client.key` — your client private key (**keep private**)

   These are issued by the server operator. The paths in the profile
(`certs\\\\ca.crt`, ...) are relative to the folder you run the EXE from.

Your `dist\\\\` folder should end up like:

```
dist\\\\
├─ vpn-client.exe
├─ wintun.dll
├─ profile.client.toml
└─ certs\\\\ { ca.crt, client.crt, client.key }
```

\---

## Run

Open a terminal **as Administrator**, then:

```bat
cd dist
vpn-client.exe -profile profile.client.toml -full-route -timeout 0
```

* `-full-route` — route **all** traffic through the tunnel (real VPN).
* `-timeout 0` — run until you press `Ctrl+C` (on exit it restores routes/DNS).
* DNS from the profile is applied to the tunnel automatically.

A `masque0` Wintun adapter appears while connected. Verify your exit IP:

```bat
curl -4 http://ifconfig.me/ip
```

It should show the **server's** IP, not your ISP's.

### Command-line flags

|Flag|Default|Meaning|
|-|-|-|
|`-profile`|(req.)|Path to the client profile TOML|
|`-full-route`|false|Route all traffic via the tunnel (real VPN)|
|`-timeout`|25s|Overall timeout; use `0` to run until Ctrl+C|
|`-verbose`|false|Verbose per-packet diagnostics (troubleshooting only)|
|`-test`|true|Test mode: route only `-test-dst` via TUN (safe on a server)|
|`-test-dst`|1.1.1.1|Test-mode destination|
|`-ping`|3|Test-mode ICMP echo count|

For a normal VPN you only need `-profile` and `-full-route` (with `-timeout 0`).

\---

## How it works

* `internal/clientcore` is platform-independent: it dials QUIC, does mTLS,
opens a CONNECT-IP session, and forwards packets between a `tun.Device` and
the tunnel. It never touches the OS routing table.
* `cmd/vpn-client/iface\\\_windows.go` does the Windows-specific part: creates the
Wintun adapter, assigns the server-issued address, installs a `0.0.0.0/0`
route plus a host route to the server via the original gateway (so QUIC does
not loop), and sets DNS on `masque0` (restored on exit).
* The same `main.go` builds for Linux via `iface\\\_linux.go` (Go build tags).

\---

## Troubleshooting

* **"wintun.dll not found"** — keep `wintun.dll` in the same folder as the EXE.
* **"Access is denied" / adapter not created** — run the terminal as
Administrator.
* **No connectivity but adapter is up** — check `server` / `server\\\_name` and
that your `certs\\\\` match the server's CA. Add `-verbose` for packet traces.
* **Build fails on `go mod download`** — ensure internet access and Go 1.21+.

\---

## Security

* Never commit real `\\\*.crt` / `\\\*.key` or a filled-in `profile.client.toml` —
`.gitignore` already excludes them.
* `client.key` is a secret; treat the whole `certs\\\\` folder as sensitive.

## Limitations

* IPv4 inside the tunnel only.
* In-tunnel DNS is plaintext UDP:53 (hidden from your local ISP, visible at the
server). DoH/DoT is future work.
* Single server/profile.

