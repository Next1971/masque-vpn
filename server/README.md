# MASQUE VPN server installation

This guide is tested on **Ubuntu 22.04**. Any modern systemd Linux with a public IP should work. All commands are run as `root` (or with `sudo`).

The server tunnels IP traffic over QUIC + HTTP/3 CONNECT-IP and authenticates clients with mutual TLS (mTLS). From **v1.3** the server also:

- sends QUIC keepalives (`KeepAlivePeriod` 15s, `MaxIdleTimeout` 3 minutes);
- pins each client certificate CN to a stable tunnel `/32` so Android can reconnect without rebuilding the TUN.

> Keep the CA private key, server private key, and client private keys out of Git and distribute client bundles only through a secure channel.

## 1. Prerequisites

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

Open the server's UDP port in your firewall and cloud security group. The default is **UDP 4433**. If you choose another port, change it consistently in the server configuration, client profiles, firewall rules, and service setup.

## 2. Get the code and build the server

```bash
git clone https://github.com/Next1971/masque-vpn-mvp.git
cd masque-vpn-mvp/android/go-src/masque-vpn-mvp
go build -trimpath -ldflags "-s -w" -o vpn-server ./cmd/poc-server
```

The binary is built from `cmd/poc-server`. On the VPS you can name it `vpn-server` or `poc-server`; the systemd unit’s `ExecStart=` must match. Common layouts are `/opt/masque/vpn-server` or `/opt/masque/bin/poc-server`. **v1.3 must replace this binary**; client reconnect is not enough if the server still hands out a new `/32` every session.

## 3. Deploy the binary

```bash
mkdir -p /opt/masque
cp vpn-server /opt/masque/vpn-server
```

## 4. Generate certificates and configuration

The generator creates the CA, the server certificate with the correct SAN, and ready-to-use client bundles for Windows and Android in one step.

```bash
cd /path/to/masque-vpn-mvp/server/scripts

# Replace the host with YOUR server's public domain or IP.
# --ip adds an extra IP to the server certificate SAN. Use it when clients may
# connect by IP even though --host is a domain.
./gen-config.sh --host vpn.example.com --ip 203.0.113.10 --clients 1
```

This produces `./out/`:

```text
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

Deliver the `windows/` bundle to Windows users and the `android/profile.masque` file to Android users over a secure channel. To add more clients later without invalidating existing ones, reuse the CA:

```bash
./gen-config.sh --host vpn.example.com --reuse-ca ./out/ca --clients 1
```

## 5. Install the systemd service

```bash
# Edit server/systemd/masque.service if your public interface is not "eth0".
# Find it with: ip route get 1.1.1.1  → the "dev XXX" name.
cp server/systemd/masque.service /etc/systemd/system/masque.service
systemctl daemon-reload
systemctl enable --now masque.service
systemctl status masque.service --no-pager
```

The unit enables IP forwarding and adds an iptables MASQUERADE rule so client traffic is NATed out to the internet.

## 6. Verify

```bash
# Should show the server listening on UDP 4433
ss -ulnp | grep 4433

# Follow logs
journalctl -u masque -f
```

You should see lines similar to `mTLS ENABLED`, `listening on 0.0.0.0:4433`, and `IP pool ... ready`.

## 7. Server configuration reference

`config.server.toml`:

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

## Troubleshooting

| Symptom | Checks |
|---|---|
| No connection | Confirm UDP 4433 is open in the VPS firewall and cloud firewall; check DNS/IP, service status, and certificate SAN |
| Service fails to start | Run `systemctl status masque.service --no-pager` and `journalctl -u masque -f` |
| Connected but no internet | Check forwarding, the interface name in `masque.service`, iptables NAT, and routes |
| TLS/mTLS error | Confirm server name, certificate chain, client certificate, and matching CA |

Before sharing logs, remove domains, IP addresses, certificates, tokens, keys, and client-profile secrets.
