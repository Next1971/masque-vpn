#!/usr/bin/env bash
# Create a CSR + private key on any OS that has OpenSSL (Git for Windows includes it).
# Upload dist.csr at developer.apple.com → Certificates → Apple Distribution.
set -euo pipefail
DIR="${1:-$(cd "$(dirname "$0")/../signing-local" && pwd)}"
mkdir -p "$DIR"
cd "$DIR"
if [[ -f dist.key ]]; then
  echo "dist.key already exists in $DIR — not overwriting"
  exit 1
fi
openssl genrsa -out dist.key 2048
openssl req -new -key dist.key -out dist.csr -subj "/CN=MASQUE Apple Distribution/C=US"
echo "Upload dist.csr in the Apple Developer portal, download the .cer, then:"
echo "  openssl x509 -in distribution.cer -inform DER -out dist.pem"
echo "  openssl pkcs12 -export -out dist.p12 -inkey dist.key -in dist.pem"
echo "(If OpenSSL 3 rejects the export, add: -legacy )"
