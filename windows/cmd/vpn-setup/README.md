# masque-setup.exe (experimental)

Windows helper that SSHes to a VPS as root and can install MASQUE or issue `profile.masque` files.

This is a **test / pre-release**. It is not a finished product. It can change the VPS (packages, firewall, systemd). There is **no certificate revocation**. Use only on a machine you can rebuild.

## Layout (required)

Keep these files in the **same folder**:

- `masque-setup.exe`
- `vpn-server-linux-amd64` **or** `vpn-server-linux-arm64` (must match the VPS CPU)

You can also choose the Linux binary in the UI. The EXE does **not** embed the server.

Supported VPS OS: Ubuntu 22.04 / 24.04 or Debian 12, as root.
