# Issuing client configs (one bundle per device)

Every device that connects gets its **own** client bundle (its own certificate).
This is not optional: the server assigns each client a unique address from the
pool (`10.8.0.0/24`) based on its certificate, and it routes return traffic by
that address. Two devices sharing the same bundle would collide on one address,
and only one of them would receive traffic.

> **Rule of thumb:** N devices → N bundles. Never reuse one bundle on two
> devices at the same time.

## Generate bundles

Configs are produced by [`server/scripts/gen-config.sh`](../server/scripts/gen-config.sh).

### First time (creates a new CA + server cert + client bundles)

```bash
cd server/scripts
./gen-config.sh --host YOUR_SERVER_HOST --ip YOUR_SERVER_IP --port 443 --clients 2
```

- `--host` — public hostname or IP that clients dial (used as TLS server name).
- `--ip` — extra IP added to the server certificate SAN (use when `--host` is a
  domain but clients may also connect by raw IP).
- `--port` — UDP port the server listens on.
- `--clients N` — how many client bundles to generate.

Output lands in `./out`:

```
out/
  ca/                 # internal CA (ca.crt + ca.key) — KEEP ca.key PRIVATE
  server/             # server.crt + server.key (deploy to the server)
  client-1/           # bundle for device #1
  client-2/           # bundle for device #2
  ...
```

Each `client-N/` contains:

- `profile.masque` — **Android** profile (self-contained: inline PEM certs). Import this single file in the app.
- `profile.client.toml` + `certs/` — **Windows/Linux** profile and its certificate files (keep them together).

## Add more devices later (reuse the SAME CA)

To issue additional bundles without re-provisioning existing clients, reuse the
existing CA. Existing devices keep working; you only hand out the **new**
bundles.

```bash
cd server/scripts
./gen-config.sh --host YOUR_SERVER_HOST --ip YOUR_SERVER_IP --port 443 \
  --reuse-ca ./out/ca --clients 8
```

This regenerates `client-1 … client-8`. The numbering restarts at 1, so take
only the new ones you need (e.g. `client-3 … client-8`) — the certificates are
still valid because they are signed by the same CA.

> Reusing the CA is what keeps previously deployed clients unaffected. If you
> create a *new* CA, every existing client stops trusting the server.

## Distribute bundles

- Send each device its own bundle **out-of-band** (not committed to git).
- **Android:** import `profile.masque` (single file). On phones you can open the
  file directly; on Android TV use the **paste-text** import (see below).
- **Windows/Linux:** copy `profile.client.toml` together with its `certs/`
  folder.

## Android TV import

Android TV boxes frequently have **no file manager**, so importing
`profile.masque` from a file may not work. Use the reliable path:

1. Open `profile.masque` (from the device's bundle) on a phone or PC and copy
   the entire contents.
2. On the TV, choose **“Import profile (paste text)”**, paste, and confirm.
3. Then **Connect**.

## Security

- **Never commit** `*.key`, keystores, `keystore.properties`, or real client
  profiles — they are git-ignored. Distribute bundles out-of-band.
- **Back up `out/ca/ca.key`.** Losing it means you can no longer issue new
  client certificates without re-provisioning every client.
