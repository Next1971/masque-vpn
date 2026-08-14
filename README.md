# MASQUE VPN

> **Status: working.** This project has been operational and tested end-to-end since **July 15, 2026** (server, Windows client, and Android client).

A minimal VPN built on the IETF **MASQUE** framework: it tunnels IP traffic
inside HTTP/3 (QUIC) using **CONNECT-IP** (RFC 9484) and authenticates both ends
with **mutual TLS (mTLS)**. Because all traffic looks like ordinary HTTP/3 over
UDP/443-style ports, it blends in with normal web traffic.

The repository contains three parts:

| Directory   | What it is                                                        |
|-------------|-------------------------------------------------------------------|
| `server/`   | Server build instructions, systemd unit, and the config generator |
| `windows/`  | Windows client (console + local web UI)                           |
| `android/`  | Android client (Kotlin app + Go core via gomobile)                |

The Go source for the server and the shared client core lives under
`android/go-src/masque-vpn-mvp/` (one Go module, `github.com/Next1971/masque-vpn-mvp`)
and is used to build both the server and the Android core.

> **Security model.** A single internal Certificate Authority (CA) signs the
> server certificate and every client certificate. The server only accepts
> clients whose certificate is signed by that CA, and each client only trusts a
> server whose certificate is signed by the same CA. **Never commit or publish
> any `*.key` file** (CA key, server key, client keys).

---

## 1. Server installation (HOWTO)

Tested on **Ubuntu 22.04** (any modern systemd Linux with a public IP works).
All commands are run as `root` (or with `sudo`).

### 1.1 Prerequisites

```bash
# Go 1.25+ (to build the server binary)
cd /tmp
curl -fsSLO https://go.dev/dl/go1.25.5.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.25.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
go version

# Build tools + openssl + iptables
apt-get update
apt-get install -y git build-essential openssl iptables
```

Open the server's UDP port in your firewall / cloud security group. The default
is **UDP 4433** (change it consistently everywhere if you pick another).

### 1.2 Get the code and build the server

```bash
git clone https://github.com/Next1971/masque-vpn-mvp.git
cd masque-vpn-mvp/android/go-src/masque-vpn-mvp
go build -trimpath -ldflags "-s -w" -o vpn-server ./cmd/poc-server
```

### 1.3 Deploy the binary

```bash
mkdir -p /opt/masque
cp vpn-server /opt/masque/vpn-server
```

### 1.4 Generate certificates and configuration

The generator creates the CA, the server certificate (with the correct SAN),
and ready-to-use client bundles for Windows and Android in one step.

```bash
cd /path/to/masque-vpn-mvp/server/scripts

# Replace the host with YOUR server's public domain or IP.
# --ip adds an extra IP to the server certificate SAN (use it when clients may
# connect by IP even though --host is a domain).
./gen-config.sh --host vpn.example.com --ip 203.0.113.10 --clients 1
```

This produces `./out/`:

```
out/
├── ca/                     internal CA  (ca.crt, ca.key)  << KEEP ca.key SECRET / BACK IT UP
├── server/                 server bundle (config.server.toml, server.crt, server.key, ca.crt)
├── windows/                Windows client bundle (profile.client.toml + certs/)
└── android/                Android client bundle (profile.masque + certs/)
```

Install the server bundle:

```bash
mkdir -p /opt/masque/cert
cp out/server/server.crt out/server/server.key out/server/ca.crt /opt/masque/cert/
cp out/server/config.server.toml /opt/masque/config.server.toml
chmod 600 /opt/masque/cert/server.key
```

> Deliver the `windows/` bundle to Windows users and the `android/profile.masque`
> file to Android users over a secure channel. To add more clients later without
> invalidating existing ones, reuse the CA:
> `./gen-config.sh --host vpn.example.com --reuse-ca ./out/ca --clients 1`

### 1.5 Install the systemd service

```bash
# Edit server/systemd/masque.service if your public interface is not "eth0"
# (find it with:  ip route get 1.1.1.1  → the "dev XXX" name).
cp server/systemd/masque.service /etc/systemd/system/masque.service
systemctl daemon-reload
systemctl enable --now masque.service
systemctl status masque.service --no-pager
```

The unit enables IP forwarding and adds an iptables MASQUERADE rule so client
traffic is NATed out to the internet.

### 1.6 Verify

```bash
# Should show the server listening on UDP 4433
ss -ulnp | grep 4433
# Follow logs
journalctl -u masque -f
```

You should see lines like `mTLS ENABLED`, `listening on 0.0.0.0:4433`, and
`IP pool ... ready`.

### 1.7 Server config reference (`config.server.toml`)

```toml
[server]
bind        = "0.0.0.0:4433"       # UDP listen address:port
server_name = "vpn.example.com"    # must match the server certificate CN/SAN

[tls]
cert      = "/opt/masque/cert/server.crt"
key       = "/opt/masque/cert/server.key"
client_ca = "/opt/masque/cert/ca.crt"   # mTLS: clients verified against this CA

[tun]
name = "masque0"
mtu  = 1400

[network]
tun_addr  = "10.8.0.1/24"    # server address on the tunnel
pool_cidr = "10.8.0.0/24"    # client address pool
route     = "0.0.0.0/0"      # route advertised to clients (0.0.0.0/0 = full tunnel)
```

---

## 2. Windows client

See [`windows/README.md`](windows/README.md) for full details. In short:

1. Get `vpn-client.exe` and `wintun.dll` from the release archive (or build with
   `windows/scripts/build.ps1`).
2. Put the `windows/` bundle from the generator next to the EXE: this gives you
   `profile.client.toml` and a `certs/` folder.
3. Run `vpn-client.exe` (no arguments) and open <http://localhost:8080> — select
   the profile, then click **CONNECT**.
   - Console mode alternative: `vpn-client.exe -profile profile.client.toml -full-route`.

**"Disable certificate verification" toggle.** Some setups fail the TLS
handshake on server certificate validation (self-signed CA, hostname mismatch).
The client offers an opt-in switch to skip that check:

- Web UI: tick **"Disable certificate verification"** before connecting.
- Console: add the `-insecure` flag.
- Profile: set `insecure = true` under `[tls]`.

It is **off by default** (secure). Only enable it while troubleshooting — it
disables authentication of the server.

---

## 3. Android client

See [`android/README-Android.md`](android/README-Android.md). In short:

1. Install the APK from the release archive (enable "install from unknown
   sources"), or build it yourself (Android SDK 34, NDK r27c, JDK 17):

   ```bash
   cd android
   # Build the Go core into an .aar:
   #   (Windows)  scripts\build-aar.bat
   #   (Linux)    see android/README-Android.md
   ./gradlew :app:assembleRelease
   ```

2. Open the app and **import a profile** (`profile.masque` from the generator).
   The APK also ships a non-production `sample-profile.masque` in its assets so
   you can see the expected format.
3. Grant the VPN permission and connect.

---

## 4. Security notes

- **Never commit secrets.** `*.key`, keystores, `keystore.properties`, and real
  client profiles are all git-ignored. Distribute client bundles out-of-band.
- **Back up the CA.** Losing `ca.key` means you can no longer issue new client
  certificates without re-provisioning every client.
- `insecure = false` is the safe default on every client. Enable it only to work
  around a certificate problem you understand.

## 5. Protocol / stack

- **QUIC + HTTP/3** transport (`quic-go`).
- **CONNECT-IP** (RFC 9484) via `connect-ip-go` for IP-level tunneling.
- **Wintun** TUN driver on Windows; Android `VpnService` file descriptor on
  Android; native TUN on Linux.
- **mTLS** with an internal EC (P-256) CA for mutual authentication.
