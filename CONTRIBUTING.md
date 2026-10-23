# Contributing to Meshbeacon

Thank you for your interest in contributing to Meshbeacon! Whether you are reporting a bug, adding discovery support for a new media service, or testing on different hardware platforms, we welcome your help.

---

## Getting Started

Meshbeacon is written in Go (1.24+). It has zero CGO dependencies and uses standard Go toolchains.

### Local Development Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/shimco/meshbeacon.git
   cd meshbeacon
   ```

2. **Run tests:**
   ```bash
   go test -v ./...
   ```

3. **Verify multi-arch builds:**
   ```bash
   # Linux x86_64
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/meshbeacon

   # Linux ARM64 (Raspberry Pi 4/5)
   GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/meshbeacon

   # Darwin Apple Silicon (macOS)
   GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/meshbeacon
   ```

---

## Development Principles

- **Zero-Allocation Data Paths:** The reverse streaming proxy uses `sync.Pool` 32KB buffers to avoid garbage collection pauses during high-bitrate video streaming. Avoid allocating buffers per-request.
- **Hardware-Aware Design:** Keep RAM footprints under 10MB and respect low-write constraints for devices booting from micro-SD cards (Raspberry Pis) or flash memory (OpenWrt routers).
- **Network Privacy:** Never commit real Tailscale MagicDNS domains, personal mesh nodes, or internal IPs to tests, fixtures, or examples. Use standard examples like `media-server.your-tailnet.ts.net`.

---

## Submitting Pull Requests

1. Fork the repo and create your branch from `main`.
2. Ensure `go test ./...` passes and `go mod tidy` produces no unexpected diffs.
3. Submit your PR using the project's [Pull Request Template](.github/PULL_REQUEST_TEMPLATE.md).
