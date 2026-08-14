# Certificates (mTLS)

This client uses mutual TLS. You must place three PEM files here:

- `ca.crt`     — CA certificate that signed the server certificate
- `client.crt` — your client certificate (issued by the same CA)
- `client.key` — your client private key (KEEP PRIVATE)

These are issued by the server operator. Do NOT commit real keys to GitHub.
The `.gitignore` already excludes `*.crt` and `*.key` in this folder.
