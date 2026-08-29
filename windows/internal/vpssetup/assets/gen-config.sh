#!/usr/bin/env bash
#
# gen-config.sh — MASQUE VPN configuration & certificate generator.
#
# Creates an internal Certificate Authority (CA), a server certificate, and one
# or more client certificates, then produces ready-to-use bundles:
#
#   out/
#   ├── ca/                     internal CA (ca.crt, ca.key)   [KEEP ca.key SECRET]
#   ├── server/                 server bundle
#   │   ├── config.server.toml  server config (points at server.crt/key + ca.crt)
#   │   ├── server.crt
#   │   ├── server.key          [SECRET]
#   │   └── ca.crt
#   ├── windows/                Windows client bundle
#   │   ├── profile.client.toml
#   │   └── certs/{ca.crt, client.crt, client.key}
#   └── android/                Android client bundle
#       ├── profile.masque      (same TOML format, ready to import in the app)
#       └── certs/{ca.crt, client.crt, client.key}
#
# mTLS model: a single internal CA signs the server cert and every client cert.
# The server verifies clients against ca.crt (client_ca); each client verifies
# the server against the same ca.crt. Keys are EC P-256 (fast, small).
#
# Usage:
#   ./gen-config.sh --host <SERVER_HOST> [options]
#
# Required:
#   --host HOST         Public server hostname or IP that clients connect to and
#                       that appears in the server certificate SAN.
#                       Example: vpn.example.com  or  203.0.113.10
#
# Options:
#   --port PORT         UDP port the server listens on (default: 4433).
#   --ip IP             Extra IP to add to the server certificate SAN
#                       (use when --host is a domain but clients may also dial by IP).
#   --clients N         Number of client bundles to generate (default: 1).
#                       With N>1, output goes to windows-1/, android-1/, ...
#   --index N           Issue a single client with CN masque-client-N (does not
#                       reuse the unnumbered masque-client CN). Implies --clients 1.
#   --android-only      Write profile.masque only (no windows/ toml + certs).
#   --client-only       Do not create a CA or server cert; requires --reuse-ca.
#                       Use this to add devices without replacing server.crt.
#   --days N            Certificate validity in days (default: 825).
#   --out DIR           Output directory (default: ./out).
#   --pool CIDR         Client address pool (default: 10.8.0.0/24).
#   --tun-addr CIDR     Server TUN address (default: 10.8.0.1/24).
#   --route CIDR        Route advertised to clients (default: 0.0.0.0/0 = full tunnel).
#   --dns "a,b"         Comma-separated DNS servers for clients (default: 1.1.1.1).
#   --reuse-ca DIR      Reuse an existing CA from DIR (must contain ca.crt + ca.key)
#                       instead of creating a new one. Use this to add more clients
#                       later without invalidating existing certificates.
#   -h, --help          Show this help.
#
# Examples:
#   ./gen-config.sh --host vpn.example.com --ip 203.0.113.10
#   ./gen-config.sh --host 203.0.113.10 --clients 3
#   ./gen-config.sh --host vpn.example.com --reuse-ca ./out/ca --clients 1
#   ./gen-config.sh --host vpn.example.com --reuse-ca ./out/ca \
#       --client-only --android-only --index 9 --out /tmp/c9
#
set -euo pipefail

# ---- defaults ---------------------------------------------------------------
HOST=""
PORT="4433"
EXTRA_IP=""
CLIENTS="1"
DAYS="825"
OUT="./out"
POOL="10.8.0.0/24"
TUN_ADDR="10.8.0.1/24"
ROUTE="0.0.0.0/0"
DNS="1.1.1.1"
REUSE_CA=""
INDEX=""
ANDROID_ONLY=0
CLIENT_ONLY=0

usage() { sed -n '2,75p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

# ---- parse args -------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --host)     HOST="$2"; shift 2 ;;
    --port)     PORT="$2"; shift 2 ;;
    --ip)       EXTRA_IP="$2"; shift 2 ;;
    --clients)  CLIENTS="$2"; shift 2 ;;
    --days)     DAYS="$2"; shift 2 ;;
    --out)      OUT="$2"; shift 2 ;;
    --pool)     POOL="$2"; shift 2 ;;
    --tun-addr) TUN_ADDR="$2"; shift 2 ;;
    --route)    ROUTE="$2"; shift 2 ;;
    --dns)      DNS="$2"; shift 2 ;;
    --reuse-ca)     REUSE_CA="$2"; shift 2 ;;
    --index)        INDEX="$2"; shift 2 ;;
    --android-only) ANDROID_ONLY=1; shift ;;
    --client-only)  CLIENT_ONLY=1; shift ;;
    -h|--help)  usage 0 ;;
    *) echo "Unknown option: $1" >&2; usage 1 ;;
  esac
done

if [ -z "$HOST" ]; then
  echo "ERROR: --host is required." >&2
  usage 1
fi
command -v openssl >/dev/null 2>&1 || { echo "ERROR: openssl not found." >&2; exit 1; }

if [ -n "$INDEX" ]; then
  [[ "$INDEX" =~ ^[1-9][0-9]*$ ]] || { echo "ERROR: --index must be a positive integer." >&2; exit 1; }
  CLIENTS="1"
fi
if [ "$CLIENT_ONLY" -eq 1 ]; then
  [ -n "$REUSE_CA" ] || { echo "ERROR: --client-only requires --reuse-ca." >&2; exit 1; }
fi

# Detect whether HOST is an IP address (so it goes into SAN as IP, not DNS).
is_ip() { [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; }

# Build a TOML dns array from a comma list: "1.1.1.1,8.8.8.8" -> "1.1.1.1", "8.8.8.8"
dns_toml() {
  local out="" first=1 d
  IFS=',' read -ra arr <<< "$1"
  for d in "${arr[@]}"; do
    d="$(echo "$d" | xargs)"   # trim
    [ -z "$d" ] && continue
    if [ $first -eq 1 ]; then out="\"$d\""; first=0; else out="$out, \"$d\""; fi
  done
  echo "$out"
}

echo "==> Output directory: $OUT"
mkdir -p "$OUT"

# ---- 1. Certificate Authority ----------------------------------------------
CADIR="$OUT/ca"
if [ "$CLIENT_ONLY" -eq 1 ]; then
  echo "==> Client-only: using CA from $REUSE_CA (server cert unchanged)"
  [ -f "$REUSE_CA/ca.crt" ] && [ -f "$REUSE_CA/ca.key" ] || { echo "ERROR: $REUSE_CA must contain ca.crt and ca.key." >&2; exit 1; }
  mkdir -p "$CADIR"
  cp "$REUSE_CA/ca.crt" "$CADIR/ca.crt"
  cp "$REUSE_CA/ca.key" "$CADIR/ca.key"
  chmod 600 "$CADIR/ca.key"
else
if [ -n "$REUSE_CA" ]; then
  echo "==> Reusing CA from: $REUSE_CA"
  [ -f "$REUSE_CA/ca.crt" ] && [ -f "$REUSE_CA/ca.key" ] || { echo "ERROR: $REUSE_CA must contain ca.crt and ca.key." >&2; exit 1; }
  mkdir -p "$CADIR"
  cp "$REUSE_CA/ca.crt" "$CADIR/ca.crt"
  cp "$REUSE_CA/ca.key" "$CADIR/ca.key"
else
  echo "==> Creating internal CA (EC P-256)"
  mkdir -p "$CADIR"
  openssl ecparam -name prime256v1 -genkey -noout -out "$CADIR/ca.key"
  openssl req -new -x509 -key "$CADIR/ca.key" -sha256 -days "$DAYS" \
    -subj "/CN=MASQUE-Internal-CA" -out "$CADIR/ca.crt"
fi
chmod 600 "$CADIR/ca.key"
fi

# ---- 2. Server certificate (with SAN) ---------------------------------------
if [ "$CLIENT_ONLY" -eq 0 ]; then
echo "==> Creating server certificate for host: $HOST"
SRVDIR="$OUT/server"
mkdir -p "$SRVDIR"

# Compose SAN entries.
SAN="DNS:localhost,IP:127.0.0.1"
if is_ip "$HOST"; then
  SAN="IP:$HOST,$SAN"
else
  SAN="DNS:$HOST,$SAN"
fi
if [ -n "$EXTRA_IP" ]; then
  SAN="$SAN,IP:$EXTRA_IP"
fi

openssl ecparam -name prime256v1 -genkey -noout -out "$SRVDIR/server.key"
openssl req -new -key "$SRVDIR/server.key" -subj "/CN=$HOST" -out "$SRVDIR/server.csr"
openssl x509 -req -in "$SRVDIR/server.csr" -CA "$CADIR/ca.crt" -CAkey "$CADIR/ca.key" \
  -CAcreateserial -sha256 -days "$DAYS" \
  -extfile <(printf "subjectAltName=%s\nextendedKeyUsage=serverAuth\n" "$SAN") \
  -out "$SRVDIR/server.crt"
rm -f "$SRVDIR/server.csr"
cp "$CADIR/ca.crt" "$SRVDIR/ca.crt"
chmod 600 "$SRVDIR/server.key"

# ---- 3. Server config -------------------------------------------------------
cat > "$SRVDIR/config.server.toml" <<TOML
# MASQUE VPN server config (connect-ip-go, mTLS).
# Generated by gen-config.sh. Paths below assume the server bundle is deployed
# to /opt/masque/ (adjust if you use a different directory).
[server]
bind        = "0.0.0.0:$PORT"
server_name = "$HOST"

[tls]
cert      = "/opt/masque/cert/server.crt"
key       = "/opt/masque/cert/server.key"
client_ca = "/opt/masque/cert/ca.crt"   # mTLS: clients are verified against this CA

[tun]
name = "masque0"
mtu  = 1400

[network]
tun_addr  = "$TUN_ADDR"
pool_cidr = "$POOL"
route     = "$ROUTE"
TOML
fi

# ---- 4. Client bundles ------------------------------------------------------
DNS_ARR="$(dns_toml "$DNS")"

make_client() {
  local idx="$1" cn windir anddir
  if [ -n "$INDEX" ]; then
    cn="masque-client-$INDEX"
    windir="$OUT/windows"
    anddir="$OUT/android"
  elif [ "$CLIENTS" -eq 1 ]; then
    cn="masque-client"
    windir="$OUT/windows"
    anddir="$OUT/android"
  else
    cn="masque-client-$idx"
    windir="$OUT/windows-$idx"
    anddir="$OUT/android-$idx"
  fi
  echo "==> Creating client certificate: $cn"

  local tmp
  tmp="$(mktemp -d)"
  openssl ecparam -name prime256v1 -genkey -noout -out "$tmp/client.key"
  openssl req -new -key "$tmp/client.key" -subj "/CN=$cn" -out "$tmp/client.csr"
  openssl x509 -req -in "$tmp/client.csr" -CA "$CADIR/ca.crt" -CAkey "$CADIR/ca.key" \
    -CAcreateserial -sha256 -days "$DAYS" \
    -extfile <(printf "extendedKeyUsage=clientAuth\n") \
    -out "$tmp/client.crt"

  if [ "$ANDROID_ONLY" -eq 0 ]; then
  # --- Windows bundle (backslash paths) ---
  mkdir -p "$windir/certs"
  cp "$CADIR/ca.crt" "$windir/certs/ca.crt"
  cp "$tmp/client.crt" "$windir/certs/client.crt"
  cp "$tmp/client.key" "$windir/certs/client.key"
  cat > "$windir/profile.client.toml" <<TOML
# MASQUE VPN — Windows client profile.
# Place this file next to vpn-client.exe; keep the certs/ folder alongside it.
[server]
server      = "$HOST:$PORT"
server_name = "$HOST"

[tls]
ca   = "certs\\\\ca.crt"
cert = "certs\\\\client.crt"
key  = "certs\\\\client.key"
# Disable server certificate verification. INSECURE — troubleshooting only.
insecure = false

[tun]
tun_name = "masque0"
mtu      = 1400
dns      = [$DNS_ARR]
TOML
  fi

  # --- Android / Windows GUI bundle (self-contained profile.masque) ---
  # The Android app imports a SELF-CONTAINED .masque profile with inline PEM
  # blocks (it has no filesystem paths to certs). Keys: [server].address/name,
  # [tun].dns, and [tls].ca/cert/key as triple-quoted PEM. See ProfileStore.kt.
  mkdir -p "$anddir/certs"
  cp "$CADIR/ca.crt" "$anddir/certs/ca.crt"
  cp "$tmp/client.crt" "$anddir/certs/client.crt"
  cp "$tmp/client.key" "$anddir/certs/client.key"

  local first_dns
  first_dns="$(echo "$DNS" | cut -d',' -f1 | xargs)"
  {
    echo "# MASQUE VPN — Android client profile. Import this file in the app."
    echo "[server]"
    echo "address = \"$HOST:$PORT\""
    echo "name    = \"$HOST\""
    echo ""
    echo "[tun]"
    echo "dns = \"$first_dns\""
    echo ""
    echo "[tls]"
    echo "ca = \"\"\""
    cat "$CADIR/ca.crt"
    echo "\"\"\""
    echo "cert = \"\"\""
    cat "$tmp/client.crt"
    echo "\"\"\""
    echo "key = \"\"\""
    cat "$tmp/client.key"
    echo "\"\"\""
  } > "$anddir/profile.masque"

  chmod 600 "$anddir/profile.masque" "$anddir/certs/client.key"
  if [ "$ANDROID_ONLY" -eq 0 ]; then
    chmod 600 "$windir/certs/client.key"
  fi
  rm -rf "$tmp"
}

for i in $(seq 1 "$CLIENTS"); do
  make_client "$i"
done

# ---- 5. Summary -------------------------------------------------------------
cat <<EOF

==============================================================================
Done. Generated under: $OUT

Server bundle  -> $OUT/server/   (deploy to /opt/masque/: put certs in
                                  /opt/masque/cert/ and config.server.toml in
                                  /opt/masque/config.server.toml)
CA             -> $OUT/ca/       (KEEP ca.key SECRET; back it up to add clients later)
Windows client -> $OUT/windows*/ (ship profile.client.toml + certs/ with vpn-client.exe)
Android client -> $OUT/android*/ (import profile.masque; certs/ travels with it)

SECURITY NOTES
  * Never commit or publish *.key files (ca.key, server.key, client.key).
  * 'insecure = false' is the safe default. Only enable it to work around a
    certificate problem you understand.
  * To add more clients later without breaking existing ones:
      ./gen-config.sh --host $HOST --reuse-ca $OUT/ca --clients 1
==============================================================================
EOF
