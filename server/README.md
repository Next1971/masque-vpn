# MASQUE VPN server

This directory contains deployment assets for the MASQUE VPN server. The project uses QUIC + HTTP/3 CONNECT-IP with mutual TLS.

> Status: experimental MVP. Review the configuration and threat model before exposing a server to real users.

## Before you start

You need:

- A Linux VPS with sudo/root access
- A public IP address and an open UDP port for QUIC
- A domain name if your certificate setup requires one
- Server binaries and configuration generated for your deployment
- Client certificates/profiles distributed only to trusted users

## Deployment flow

1. Copy the server binary and its configuration to the VPS.
2. Create or install the TLS material required by the server and clients. Keep private keys outside Git.
3. Configure the listening address, UDP port, routing/NAT, and allowed client identities.
4. Open the selected UDP port in the VPS firewall and provider firewall.
5. Install the systemd unit from `systemd/` and enable it.
6. Verify the service status and logs.
7. Create a client profile and test it from Windows or Android.

## systemd

The `systemd/` directory is intended for service-unit files and deployment-related configuration. Adapt paths, user/group, ports, and environment variables to your server before enabling a unit.

Useful checks after installation:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now masque-vpn
sudo systemctl status masque-vpn
sudo journalctl -u masque-vpn -f
```

Use the actual unit name supplied by your deployment files if it differs from `masque-vpn`.

## Firewall and network checks

- Allow the configured UDP port; QUIC does not use TCP for its transport.
- Ensure IP forwarding and NAT are configured if the server forwards client traffic to the internet.
- Confirm that the server certificate name matches the endpoint used by clients.
- Do not commit certificates, private keys, client profiles containing secrets, or production IP addresses.

## Troubleshooting

| Symptom | First checks |
|---|---|
| Client cannot connect | UDP port, DNS/IP, server process, certificate name and client identity |
| Service will not start | `systemctl status` and `journalctl -u masque-vpn` |
| Connected but no traffic | IP forwarding, routes, firewall forwarding rules, NAT |
| TLS or mTLS error | Certificate chain, expiry, server name, client certificate/key pairing |

When reporting a problem, remove IP addresses, domains, certificates, tokens, and other secrets from logs.
