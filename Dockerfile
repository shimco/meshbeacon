# Multi-stage Docker build for Meshbeacon
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy dependency manifests and source
COPY go.mod ./
COPY go.sum* ./
COPY . .

# Download dependencies and tidy
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /meshbeacon ./cmd/meshbeacon

# Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /meshbeacon /usr/local/bin/meshbeacon

# Ports: Jellyfin (8096/TCP, 7359/UDP), Plex (32400/TCP, 32414/UDP), mDNS (5353/UDP)
EXPOSE 8096/tcp 32400/tcp 7359/udp 32414/udp 5353/udp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD /usr/local/bin/meshbeacon -health-check -config /etc/meshbeacon/meshbeacon.json 2>/dev/null || /usr/local/bin/meshbeacon -health-check || exit 1

ENTRYPOINT ["meshbeacon"]
