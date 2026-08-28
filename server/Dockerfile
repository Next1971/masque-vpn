# MASQUE VPN server — multi-stage build.
# Run with host networking (or publish UDP) + NET_ADMIN + /dev/net/tun.
# Certificates and config are mounted at runtime; nothing secret is baked in.
FROM golang:1.25.5-bookworm AS build
WORKDIR /src
COPY android/go-src/masque-vpn/go.mod android/go-src/masque-vpn/go.sum ./
RUN go mod download
COPY android/go-src/masque-vpn/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/vpn-server ./cmd/poc-server

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends iptables iproute2 ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/vpn-server /usr/local/bin/vpn-server
COPY server/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/vpn-server
WORKDIR /opt/masque
EXPOSE 443/udp
ENTRYPOINT ["/entrypoint.sh"]
