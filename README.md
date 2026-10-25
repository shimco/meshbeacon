# Meshbeacon

> **Smart TV discovery beacon (UDP/mDNS), stream gateway, and local artwork cache for Jellyfin, Plex & Tailscale/WireGuard meshes.**  
> *Stream media to Smart TVs and family devices with native auto-discovery and zero client VPNs.*

[![CI](https://github.com/shimco/meshbeacon/actions/workflows/ci.yml/badge.svg)](https://github.com/shimco/meshbeacon/actions/workflows/ci.yml)
[![Docker](https://github.com/shimco/meshbeacon/actions/workflows/docker.yml/badge.svg)](https://github.com/shimco/meshbeacon/actions/workflows/docker.yml)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fshimco%2Fmeshbeacon-2496ED?logo=docker&logoColor=white)](https://github.com/shimco/meshbeacon/pkgs/container/meshbeacon)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shimco/meshbeacon)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Binary Size](https://img.shields.io/badge/Binary-<7MB-success.svg)](https://github.com/shimco/meshbeacon/releases)

---

## The Problem

Running VPN clients directly on Smart TVs (Google TV, Android TV, Apple TV, Fire TV, Roku) causes chronic friction:
1. **Aggressive Standby Sleep:** Android TV and Smart TV platforms aggressively kill background VPN processes to conserve memory during standby. Waking the TV leaves tunnels like Tailscale dead and media apps buffering indefinitely.
2. **Family Support Overhead:** Explaining VPN client toggles, MFA, or private IP configuration to non-technical partners, kids, or parents is an ongoing support burden.
3. **Plex Relay Throttling:** Without local network presence, Plex falls back to its 2 Mbps internet relay, forcing heavy video transcoding on your server.

## The Solution

**Meshbeacon** is a lightweight local LAN gateway and discovery beacon. You run it on a single machine on your local network (a Raspberry Pi, mini-PC, NAS, home server, or OpenWrt travel router).

Meshbeacon bridges your remote server into your local home Wi-Fi:
* **Native Local Auto-Discovery:** Emulates local server discovery probes (UDP `7359` for Jellyfin, GDM UDP `32414` for Plex, and mDNS/DNS-SD `_jellyfin._tcp`) so TV apps find the server automatically.
* **Zero Client Installs:** TVs, streaming sticks, and family devices connect directly over standard local Wi-Fi without VPN software.
* **Zero-Allocation 4K Streaming:** Hardened reverse proxy using pooled 32KB buffers (`sync.Pool`) delivers high-throughput video streaming with near-zero CPU and <10MB of RAM.
* **Dual-Tier Asset Caching:** Stores posters and photo thumbnails in RAM or local NVMe with zero SD-card wear, making browsing fast over high-latency WAN links.

---

## Architecture

```text
 ┌────────────────────────────────────────────────────────┐
 │              Offsite Media Server / NAS                │
 │    (Plex, Jellyfin, Jellyseerr, Immich, Navidrome)     │
 └───────────────────────────┬────────────────────────────┘
                             │
                      Encrypted Overlay
          (Tailscale / WireGuard / ZeroTier / Nebula)
                             │
                             ▼
 ┌────────────────────────────────────────────────────────┐
 │                 Meshbeacon Gateway                     │
 │          (Raspberry Pi / Home Server / Docker)         │
 │                                                        │
 │  ┌───────────────────────┐   ┌───────────────────────┐ │
 │  │   Streaming Proxy     │   │   Discovery Beacon    │ │
 │  │   (HTTP/WebSockets)   │   │  (UDP 7359, GDM, mDNS)│ │
 │  └───────────────────────┘   └───────────────────────┘ │
 └───────────────────────────┬────────────────────────────┘
                             │
                       Local Home LAN
                      (Wi-Fi / Ethernet)
                             │
       ┌─────────────────────┼─────────────────────┐
       ▼                     ▼                     ▼
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│   Smart TV   │      │   Apple TV   │      │    iPads     │
│  (No VPN)    │      │  (Stock App) │      │  (Guests)    │
└──────────────┘      └──────────────┘      └──────────────┘
```

---

## Quick Start

### Docker Compose (Recommended)

1. Create `meshbeacon.json` pointing to your remote server:
   ```json
   {
     "advertise_addr": "auto",
     "services": [
       {
         "name": "jellyfin",
         "local_port": 8096,
         "upstream_url": "http://media-server.your-tailnet.ts.net:8096",
         "discovery": { "type": "jellyfin", "port": 7359 }
       },
       {
         "name": "plex",
         "local_port": 32400,
         "upstream_url": "http://media-server.your-tailnet.ts.net:32400",
         "discovery": { "type": "plex", "port": 32414 }
       }
     ]
   }
   ```

2. Run with `docker-compose.yml`:
   ```yaml
   services:
     meshbeacon:
       image: ghcr.io/shimco/meshbeacon:latest
       container_name: meshbeacon
       restart: unless-stopped
       network_mode: host
       command: ["-config", "/etc/meshbeacon/meshbeacon.json"]
       volumes:
         - ./meshbeacon.json:/etc/meshbeacon/meshbeacon.json:ro
   ```

3. Launch:
   ```bash
   docker compose up -d
   ```

### Homebrew (macOS & Linux)

```bash
brew tap shimco/tap
brew install meshbeacon
brew services start meshbeacon
```

*(For Debian/Ubuntu `.deb`, Raspberry Pi automated script, or OpenWrt travel routers, see the [Getting Started Guide](docs/getting-started.md)).*

---

## Documentation & Deep Dives

| Guide | Description |
| :--- | :--- |
| **[Getting Started & Installation](docs/getting-started.md)** | Step-by-step guides for Docker, Homebrew, Debian/Ubuntu `.deb`, standalone script, OpenWrt, plus configuration reference. |
| **[Plex Direct Play Resolution](docs/plex-direct-play.md)** | Resolving Custom Access URLs and LAN subnets to bypass the 2 Mbps Plex Relay and force full quality Direct Play. |
| **[Hardware Caching Strategy](docs/caching-strategy.md)** | Memory auto-scaling brackets (8MB on micro-routers to 512MB on servers), NVMe persistence, and zero flash wear design. |
| **[Why Meshbeacon? Comparing Architectural Alternatives](docs/alternatives-comparison.md)** | Detailed technical comparisons against Squid caching, Tailscale Services, and pairing with AdGuard Home. |
| **[Smart TV Tips & Troubleshooting](docs/smart-tv-troubleshooting.md)** | Resolving Wi-Fi multicast blocking, IGMP snooping, DHCP reservations, and Apple TV Swiftfin permissions. |
| **[Multi-Server Proxying & Federation](docs/multi-server-federation.md)** | Exposing multiple Jellyfin or Plex servers on separate ports across the same living room Wi-Fi. |

---

## Core Capabilities

- [x] **Jellyfin LAN Discovery:** Listens for broadcast probes (`Who is JellyfinServer?`) on UDP port `7359` and actively announces presence across subnets.
- [x] **Plex GDM Discovery:** Responds to Plex "Good Day Mate" UDP discovery queries on port `32414`.
- [x] **mDNS / DNS-SD Broadcasting:** Publishes `_jellyfin._tcp` and `_plexmediasvr._tcp` zero-conf records for Apple TV (Swiftfin), Infuse, and iOS clients.
- [x] **Zero-Config Identity Detection:** Automatically queries upstream server endpoints (`/System/Info/Public` and `/identity`) to fetch Server IDs and names (no manual UUID configuration needed).
- [x] **Zero-Allocation 4K Streaming:** Hardened Go reverse proxy with pooled 32KB buffers (`sync.Pool`) for high-bitrate streaming with minimal RAM usage.
- [x] **Dual-Tier Caching:** In-memory LRU cache scaled to hardware (8MB to 512MB) to eliminate SD card wear, with an optional persistent SSD/NVMe tier.
- [x] **Extensible Service Proxy:** Stream and cache companion apps like Jellyseerr, Overseerr, Immich, or Audiobookshelf.
- [x] **Cross-Platform Static Binaries:** Pure Go (`CGO_ENABLED=0`) with zero external libc runtime dependencies (x86_64, ARM64, ARMv7, MIPS/MIPSEL for OpenWrt).

---

## Hardware Compatibility & Tested Environments

Meshbeacon is written in pure Go with zero runtime dependencies. Here is the verified hardware status:

| Environment | Hardware / OS | Architecture | Status | Validation Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Linux Server** | Ubuntu Server 24.04 LTS | `amd64` (x86_64) | 🟢 **Verified** | Bare-metal (`systemd`), APT package (`.deb`), and Docker Compose |
| **Apple Mac** | macOS 15 Sequoia | `arm64` (Apple Silicon) | 🟢 **Verified** | Standalone binary and Homebrew Tap (`brew services` background daemon) |
| **Docker / Compose** | Docker 26+ / Compose 2.26+ | `amd64`, `arm64` | 🟢 **Verified** | Official multi-arch GHCR image with self-healing healthchecks |
| **Raspberry Pi** | Raspberry Pi 4 (Debian 12 / RPi OS) | `arm64` | 🟡 **Active Testing** | Tested over Tailscale mesh network |
| **OpenWrt Router** | MediaTek Filogic / GL.iNet (Beryl AX / Flint) | `arm64` / `mips` | 🟡 **Community Testing** | POSIX procd service installer tested; seeking community test reports |

> [!TIP]
> **Help Validate Other Hardware!**  
> If you are running Meshbeacon on an OpenWrt router, Orange Pi, Rockchip SBC, Proxmox LXC, or an older Raspberry Pi, please [open a Discussion](https://github.com/shimco/meshbeacon/discussions) or Issue with your test notes. I would love to add your hardware setup to the verified list!

---

## Privacy & Zero Telemetry

Meshbeacon is 100% private and offline-first:
* **Zero external network calls**: It never phones home, collects analytics, or checks external licensing servers.
* **Low-wear logging**: Access logs are kept in a local in-memory RAM ring buffer by default to prevent burning out SD cards and flash storage.
* All video traffic and metadata travel strictly between your local devices and your private upstream server.

---

## Feedback & Issues

Found a bug, have a feature idea, or running this on a travel router? [Open a GitHub Issue or Discussion](https://github.com/shimco/meshbeacon/issues). Real-world feedback and test notes are very welcome!

---

## License

Distributed under the [MIT License](LICENSE). Copyright (c) 2026 Graham Shimmin.
