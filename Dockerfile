# Multi-stage Docker build for Meshbeacon
FROM golang:1.24-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary for target architecture
RUN if [ -n "$TARGETARCH" ]; then export GOARCH="$TARGETARCH"; fi && \
    if [ "$TARGETARCH" = "arm" ] && [ -n "$TARGETVARIANT" ]; then export GOARM="${TARGETVARIANT#v}"; fi && \
    CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" go build -ldflags="-w -s" -o /meshbeacon ./cmd/meshbeacon

# Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /meshbeacon /usr/local/bin/meshbeacon

# Ports: Jellyfin (8096/TCP, 7359/UDP), Plex (32400/TCP, 32414/UDP), mDNS (5353/UDP)
EXPOSE 8096/tcp 32400/tcp 7359/udp 32414/udp 5353/udp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD /usr/local/bin/meshbeacon -health-check -config /etc/meshbeacon/meshbeacon.json 2>/dev/null || /usr/local/bin/meshbeacon -health-check || exit 1

ENTRYPOINT ["meshbeacon"]
