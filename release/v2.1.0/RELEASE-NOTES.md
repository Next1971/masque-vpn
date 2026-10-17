v2.1.0 is GitHub **Latest**. It replaces [v1.7.1](https://github.com/Next1971/masque-vpn/releases/tag/v1.7.1).

The published MASQUE server had never carried a TCP tunnel, and it had never answered someone who simply opened the address in a browser. v2.0.0 was a pre-release experiment: it listened on TCP, then closed the handshake unless the client already had a certificate. v2.1.0 keeps the tunnel and serves a website to everyone else.

## What changed

- **UDP** is the same CONNECT-IP session as before. A client certificate from the private CA is required. There is no website on UDP.
- **TCP** on the same port asks for a client certificate and does not require one. A valid certificate and an HTTP/2 CONNECT-IP request open the VPN. A browser, a probe, a missing certificate, or a wrong certificate gets an HTML page.
- The server certificate is still the private CA. A browser shows a warning, then the page.
- Android phone, Android TV, and Windows: transport (Automatic, UDP only, or TCP only), MTU (default 1340), kill switch (default off), day and month traffic, and ping on a TCP session.
- **iOS 2.0** is on TestFlight: [join/x52N41V1](https://testflight.apple.com/join/x52N41V1). No IPA. It uses the private CA and works with this server over UDP and TCP.

Profiles and certificates do not change. An existing 2.0 server can be replaced with this binary without issuing new client bundles.

## Compatibility

- A **v1.7.1** (and older UDP) client still reaches a **v2.1.0** server over UDP.
- A **v2.0.0** or **v2.1.0** client reaches this server over UDP and over TCP.
- A **v2.1.0** client still reaches an older server over UDP. TCP needs a 2.0.0 or 2.1.0 server.
- [iOS 2.0](https://testflight.apple.com/join/x52N41V1) works with this server. The older TestFlight 1.7 build is UDP-only.

## Install the server

- `install.sh` on Ubuntu or Debian, as root, with `MASQUE_HOST` and `MASQUE_PORT`. It downloads this release and checks `SHA256SUMS`. It refuses to replace an existing CA, config, or service.
- `masque-setup.exe` from Windows. Put `vpn-server-linux-amd64` or `vpn-server-linux-arm64` in the same folder. The wizard installs UDP and TCP, an optional second port, and shows a profile QR.
- Docker: `masque-docker-2.1.0.zip` (`Dockerfile`, `docker-compose.yml`, `entrypoint.sh`). Image tag `masque-vpn-server:2.1.0`. Host network, `NET_ADMIN`, and `/dev/net/tun` are required. The image is not attached as a tarball.

## Files

- `masque-phone-2.1.0.apk` — Android phone
- `masque-tv-2.1.0.apk` — Android TV
- `masque-2.1.0.msi` — Windows VPN client
- `masque-setup.exe` — Windows VPS installer
- `vpn-server-linux-amd64`, `vpn-server-linux-arm64`
- `install.sh`, `gen-config.sh`, `SHA256SUMS`
- `masque-docker-2.1.0.zip`
