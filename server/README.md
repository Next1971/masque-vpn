# MASQUE VPN server installation

This guide describes the v2.1.0 server. HTTP/3 CONNECT-IP over QUIC/UDP is the normal tunnel; HTTP/2 CONNECT-IP over TLS/TCP uses the same port number. VPN access requires a valid client certificate.

On TCP, a valid client certificate plus an HTTP/2 CONNECT-IP request opens the VPN. Browsers and connections without a valid client certificate receive an HTML page, not tunnel access. The server certificate is signed by the private CA, so a browser shows a certificate warning. UDP does not serve a website.

Experimental software; no independent security audit. Review privileged commands before use.

## Choose a deployment path

- Manual installation below: Linux VPS with systemd, a public address, and /dev/net/tun. The manual guide was tested on Ubuntu 22.04.
- install.sh from [v2.1.0](../../releases/tag/v2.1.0): fresh Ubuntu/Debian installation as root, with MASQUE_HOST and MASQUE_PORT. It checks SHA256SUMS and refuses to overwrite an existing CA, configuration, or service. Review the script before running it; it is not an in-place upgrade command.
- [Windows VPS Setup Helper](../windows/README.md#install-the-server-from-windows-masque-setupexe): experimental; Ubuntu 22.04/24.04/26.04 or Debian 12. The v2.1.0 wizard deploys UDP and TCP, an optional second port, and displays a profile QR.
- [Docker](#docker): experimental Linux-host deployment.

For an existing installation, preserve the CA, certificates, profiles, and configuration. Inspect the actual systemd ExecStart path before replacing its binary; do not generate a new CA merely to upgrade. A v2.0.0 server can be replaced with v2.1.0 without issuing new profiles. Back up credentials/configuration securely and plan a maintenance window.

## 1. Prerequisites and network access

Run administrative commands as root or with sudo. Install git, build-essential, openssl, and iptables through the distribution package manager. A source build requires Go 1.25.5 or later. Prebuilt binaries do not require a Go compiler.

Allow the configured port separately for UDP and TCP in both the host firewall and cloud security group. This guide uses 4433 as an example; use the actual port from your configuration and profiles.

### UDP socket buffers

From the repository root:

```bash
cp server/sysctl/99-masque-udp.conf /etc/sysctl.d/99-masque-udp.conf
sysctl --system
sysctl net.core.rmem_max net.core.wmem_max
# Reference ceiling for both: 67108864 (64 MiB)
```

These ceilings help QUIC avoid kernel queue drops under load. The supplied systemd unit also applies them at startup. They are not TCP tuning or a guarantee of throughput.

## 2. Obtain the server

Download vpn-server-linux-amd64 or vpn-server-linux-arm64 for the VPS architecture from [v2.1.0](../../releases/tag/v2.1.0), and check the matching SHA256SUMS entry before deployment. The binary contains no certificates or keys.

To build the tagged release instead:

```bash
git clone --branch v2.1.0 --depth 1 https://github.com/Next1971/masque-vpn.git
cd masque-vpn/android/go-src/masque-vpn
go build -trimpath -ldflags "-s -w" -o vpn-server ./cmd/poc-server
```

For a fresh manual installation, put the resulting binary at the path used by the supplied unit:

```bash
mkdir -p /opt/masque
install -m 0755 vpn-server /opt/masque/vpn-server
```

For a downloaded binary, substitute its actual filename. Do not overwrite a running binary as an unplanned upgrade; stop the service and preserve a rollback copy first.

## 3. Generate certificates and configuration

For a fresh installation, from server/scripts/:

```bash
./gen-config.sh --host vpn.example.com --ip 203.0.113.10 --clients 1
```

Replace the example host and IP. The server identity must match its certificate SAN. The generator produces out/ca/, out/server/, and client bundles. Keep ca.key secret and backed up. Issue a separate bundle for every device; see [client configurations](../docs/CLIENTS.md).

```bash
mkdir -p /opt/masque/cert
cp out/server/server.crt out/server/server.key out/server/ca.crt /opt/masque/cert/
cp out/server/config.server.toml /opt/masque/config.server.toml
chmod 600 /opt/masque/cert/server.key
```

Transfer profile.masque privately for Android, Windows, or iOS. To issue additional credentials under the same CA, reuse the original CA rather than regenerating it:

```bash
./gen-config.sh --host vpn.example.com --reuse-ca ./out/ca --clients 1
```

The CA directory must be the original one. Check the generator's output and the client guide before distributing new bundles.

## 4. Install the service

From the repository root, inspect the unit first. Check ExecStart, configuration paths, tunnel settings, and the WAN interface used by its NAT rules. Find the WAN interface with ip route get 1.1.1.1.

```bash
cp server/systemd/masque.service /etc/systemd/system/masque.service
systemctl daemon-reload
systemctl enable --now masque.service
systemctl status masque.service --no-pager
```

The supplied unit configures forwarding, UDP buffer ceilings, and NAT. Graceful shutdown is supported on SIGTERM/SIGINT.

## Docker

Use a Linux host with Docker Engine. Host networking, NET_ADMIN, and /dev/net/tun are required. Docker Desktop is not the reference deployment.

The release provides masque-docker-2.1.0.zip with Docker sources, not a prebuilt image tarball. The documented image tag is masque-vpn-server:2.1.0. Build against the matching release tree.

After generating configuration, from the repository root, adjust the source paths below if your out/ directory is under server/scripts/:

```bash
mkdir -p server/data/cert
cp server/scripts/out/server/config.server.toml server/data/
cp server/scripts/out/server/server.crt server/scripts/out/server/server.key server/scripts/out/server/ca.crt server/data/cert/
chmod 600 server/data/cert/server.key
docker compose -f server/docker-compose.yml up -d --build
docker logs -f masque
```

The entrypoint configures forwarding, UDP buffers, and NAT on the default WAN interface; MASQUE_WAN_IF overrides the interface. Mount configuration and certificates separately; do not bake credentials into the image.

## 5. Verify both transports

For the example port 4433:

```bash
ss -ulnp | grep ':4433'
ss -tlnp | grep ':4433'
journalctl -u masque -f
```

Check the actual configured port, not just the example. Listening locally does not prove the cloud firewall allows traffic. Test an authenticated client over UDP and TCP separately, then verify the exit address, DNS, and application traffic. The TCP page alone is not a VPN test.

## 6. Configuration reference

```toml
[server]
bind = "0.0.0.0:4433" # UDP and TCP port for this example
server_name = "vpn.example.com"

[tls]
cert = "/opt/masque/cert/server.crt"
key = "/opt/masque/cert/server.key"
client_ca = "/opt/masque/cert/ca.crt"
# blocked_cns = ["masque-client-7"]

[tun]
name = "masque0"
mtu = 1369

[network]
tun_addr = "10.8.0.1/24"
pool_cidr = "10.8.0.0/24"
route = "0.0.0.0/0"
# Optional IPv6 inside the tunnel:
# tun_addr_v6 = "fd00:8::1/64"
# pool_cidr_v6 = "fd00:8::/64"
# route_v6 = "::/0"
```

Server TUN MTU is separate from the v2.1.0 Android/TV/Windows client default of 1340. Preserve the values appropriate for the deployed configuration.

IPv6 transport to the VPS and IPv6 inside the tunnel are separate. QUIC over IPv6 is supported in the newer client line; the old IPv4-only warning is not a blanket restriction on current clients. IPv6 inside the tunnel requires WAN IPv6, IPv6 forwarding, matching client routes, and appropriate NAT66/firewall rules. Account for older clients before publishing AAAA records.

### UDP 443 blocked on the client path

First test TCP only with a compatible server. Another option is an alternate external port mapped to the same listener. For example, with the process bound to 443, allow 2053 in both firewalls and redirect both transports if both are needed:

```bash
ufw allow 2053/udp
ufw allow 2053/tcp
iptables -t nat -C PREROUTING -p udp --dport 2053 -j REDIRECT --to-ports 443 2>/dev/null \
  || iptables -t nat -A PREROUTING -p udp --dport 2053 -j REDIRECT --to-ports 443
iptables -t nat -C PREROUTING -p tcp --dport 2053 -j REDIRECT --to-ports 443 2>/dev/null \
  || iptables -t nat -A PREROUTING -p tcp --dport 2053 -j REDIRECT --to-ports 443
```

These iptables commands describe IPv4 redirection. Configure and test IPv6 separately if required. Persist firewall rules using the mechanism appropriate for your VPS. Do not flush unrelated firewall rules.

Use the reachable external port in the profile, or generate optional alt_port with gen-config.sh --alt-port 2053. Keep server_name matching the certificate; a port change does not change TLS identity.

### Revoking a client CN

The server denylist is not a PKI CRL. Add the exact CN and restart during a maintenance window:

```bash
printf '%s\n' 'masque-client-7' >> /opt/masque/blocked_cns
chmod 0644 /opt/masque/blocked_cns
systemctl restart masque.service
```

The process reads the file at startup. Restart interrupts existing sessions. Certificate files are not deleted.

## Troubleshooting

| Symptom | Check |
|---|---|
| No connection | Configured port, UDP/TCP listener, host and cloud firewall, selected client transport |
| TCP page but no VPN | Client certificate, CA, CONNECT-IP request, blocked CN; page access is not tunnel authorization |
| Certificate failure | Server identity/SAN, CA, client credentials, validity dates and clock |
| Connected but no internet | Forwarding, WAN interface, NAT, routes, DNS |
| QUIC loss or poor throughput | UDP buffer ceilings, MTU, path conditions; compare TCP separately |
| Service fails | systemctl status masque.service and journalctl -u masque |

Keep insecure = false. Remove credentials and identifying information from shared logs. See [security policy](../SECURITY.md) and [roadmap](../docs/ROADMAP.md).
