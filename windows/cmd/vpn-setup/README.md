# masque-setup.exe (experimental)

Windows helper that SSHes to a VPS as root and can install MASQUE or issue `profile.masque` files.

This is a **test / experimental** helper, not a finished product. It can change the VPS (packages, firewall, systemd). Compromised client CNs can be **revoked** (append to `/opt/masque/blocked_cns` and restart `masque.service`). Use only on a machine you can rebuild.

**v2.1 wizard (this tree):** one listen port for UDP and TCP, plus an optional second port. The second port is redirected to the listen port for both protocols and written into profiles as `alt_port`. UFW, when installed, allows UDP and TCP on each chosen port. If UDP or TCP is already listening, Confirm asks whether to continue or pick another port. An existing CA, `config.server.toml`, or `masque.service` stops the install; certificates, `/opt/masque/clients`, and `blocked_cns` are not replaced. Public host is still IPv4 or DNS (no IPv6 literal; use `gen-config.sh --dial` for that).

After **Issue next bundle** or **Save bootstrap profile**, the wizard also shows a QR of that `profile.masque`. The Android phone app and iOS scan it. Android TV and the Windows client still use the file. The code contains the client private key.

The published `masque-setup.exe` before this build is still the one-port v1.5.6 wizard.

## Layout (required)

Keep these files in the **same folder**:

- `masque-setup.exe`
- `vpn-server-linux-amd64` **or** `vpn-server-linux-arm64` (must match the VPS CPU)

You can also choose the Linux binary in the UI. The EXE does **not** embed the server.

Supported VPS OS: Ubuntu 22.04 / 24.04 / 26.04 or Debian 12, as root.
