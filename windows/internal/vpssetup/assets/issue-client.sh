#!/bin/bash
# Issue one profile.masque (CN masque-client-N) without touching server.crt.
# Expects /opt/masque/ca, config.server.toml, gen-config.sh, state/next_index.
set -euo pipefail

ROOT=/opt/masque
GEN="${ROOT}/gen-config.sh"
CA="${ROOT}/ca"
CFG="${ROOT}/config.server.toml"
STATE="${ROOT}/state/next_index"

[[ -f "$GEN" ]] || { echo "ERROR: missing $GEN" >&2; exit 1; }
[[ -f "$CA/ca.key" ]] || { echo "ERROR: missing CA at $CA" >&2; exit 1; }
[[ -f "$CFG" ]] || { echo "ERROR: missing $CFG" >&2; exit 1; }

HOST=$(awk -F'"' '/^server_name/ {print $2; exit}' "$CFG")
BIND=$(awk -F'"' '/^bind/ {print $2; exit}' "$CFG")
PORT="${BIND##*:}"
[[ -n "$HOST" && "$PORT" =~ ^[0-9]+$ ]] || { echo "ERROR: cannot parse host/port from $CFG" >&2; exit 1; }

install -d -m 0755 "${ROOT}/state" "${ROOT}/clients"
if [[ ! -f "$STATE" ]]; then
  echo 9 > "$STATE"
fi
NEXT=$(tr -d '[:space:]' < "$STATE")
[[ "$NEXT" =~ ^[1-9][0-9]*$ ]] || { echo "ERROR: bad next_index" >&2; exit 1; }

# #1–8 reserved (test). App slots: 9 .. 261 (253 bundles).
if (( NEXT < 9 )); then
  echo "ERROR: next_index $NEXT < 9 (refusing to overlap test CN 1–8)" >&2
  exit 1
fi
if (( NEXT > 261 )); then
  echo "ERROR: app bundle slots full (253 issued from #9)" >&2
  exit 1
fi

OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

bash "$GEN" --host "$HOST" --port "$PORT" --reuse-ca "$CA" \
  --client-only --android-only --index "$NEXT" --out "$OUT"

install -d -m 0755 "${ROOT}/clients/${NEXT}"
install -m 0600 "$OUT/android/profile.masque" "${ROOT}/clients/${NEXT}/profile.masque"
echo $((NEXT + 1)) > "$STATE"
echo "ISSUED ${NEXT}"
