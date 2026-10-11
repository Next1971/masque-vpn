v2.0.0 is a **full experimental** GitHub pre-release. It does **not** replace **Latest**. Daily driver stays **[v1.7.1](https://github.com/Next1971/masque-vpn/releases/tag/v1.7.1)**.

If the current VPN already works, you do not need to install 2.0.0.

## What this tag is

- Tunnel MTU **1340** on Android phone, Android TV, and Windows. The client forces 1340. No MTU field and no TCP/UDP switch.
- UDP is the normal path. TCP HTTP/2 CONNECT-IP opens only when a short UDP probe to the server gets no reply. TCP uses the same host and the same ports (profile port, then optional `alt_port`).
- One server process listens on UDP and TCP on the same bind port. Profiles and certificates do not change.
- Android `2.0.0` (`versionCode` 25, TV `2.0.0-tv`). Windows product **2.0.0**.
- New Linux server binaries, `install.sh`, `gen-config.sh`, `masque-setup.exe`, and Docker sources (`masque-docker-2.0.0.zip`, image tag `masque-vpn-server:2.0.0`).

## Who should try it

- A path where UDP to the server is dropped and TCP 443 still works.
- A path where large UDP datagrams die (phone sweep: UDP empty at MTU 1365 and 1369, usable at 1340).

Everyone else: stay on **v1.7.1**.

## Compatibility

- A **v1.7.1** (and older UDP) client still reaches a **v2.0.0** server. The UDP listener is the same CONNECT-IP session.
- A **v2.0.0** client still reaches an older server over UDP. TCP fallback needs this 2.0.0 server: an old server has no HTTP/2 CONNECT-IP listener.
- iOS TestFlight **v1.7** is unchanged and stays UDP-only. No IPA is attached.

## Docker

Build on a Linux host from this tag (Compose context is the repository root):

```bash
docker compose -f server/docker-compose.yml build
```

`masque-docker-2.0.0.zip` is those same sources (`Dockerfile`, `docker-compose.yml`, `entrypoint.sh`). Host network, `NET_ADMIN`, and `/dev/net/tun` are required. The image is not attached as a tarball.

## Files

- `masque-phone-2.0.0.apk` — Android phone
- `masque-tv-2.0.0.apk` — Android TV
- `masque-2.0.0.msi` — Windows VPN client
- `masque-setup.exe` — experimental VPS installer
- `vpn-server-linux-amd64`, `vpn-server-linux-arm64`
- `install.sh`, `gen-config.sh`, `SHA256SUMS`
- `masque-docker-2.0.0.zip`

## Notes

- Server TUN MTU stays 1369. Client packets are 1340, so they fit.
- Experimental, self-hosted software. No third-party security audit.
