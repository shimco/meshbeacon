# Multi-stage Docker build for Tailbeacon
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod ./
# COPY go.sum ./
RUN go mod download || true

# Copy source
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /tailbeacon ./cmd/tailbeacon

# Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /tailbeacon /usr/local/bin/tailbeacon

EXPOSE 8096/tcp
EXPOSE 7359/udp

ENTRYPOINT ["tailbeacon"]
