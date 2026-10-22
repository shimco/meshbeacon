# 🗼 Tailbeacon

> **Broadcast remote Tailscale media services into your local LAN with native auto-discovery.**  
> *Stream Jellyfin, Plex, and Navidrome to Smart TVs, consoles, and family devices with zero VPN client apps.*

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/shimco/tailbeacon)](https://goreportcard.com/report/github.com/shimco/tailbeacon)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)](docker-compose.yml)

---

## 💡 The Problem

Running Tailscale directly on Smart TVs (Android TV, Google TV, Apple TV, Roku, Fire TV, LG webOS, Samsung Tizen) suffers from three chronic friction points:
1. **Standby Timeouts & Task Killers:** Smart TV operating systems aggressively sleep background VPN services to save memory. When waking the TV from standby, the WireGuard UDP tunnel hangs, leaving media apps buffering indefinitely.
2. **Family & Guest Friction:** Setting up Tailscale on every tablet, phone, and living room TV for family members or non-technical parents is painful to maintain and support.
3. **Hardware Overhead:** Underpowered smart TV chipsets struggle with background cryptographic packet processing during high-bitrate 4K streaming.

## 🚀 The Solution: Tailbeacon

**Tailbeacon** is a lightweight local LAN gateway and discovery beacon. You run it on a single machine on your local network (a Raspberry Pi, mini-PC, OpenWrt router, or home server). 

Tailbeacon bridges your remote Tailscale media server into your local home Wi-Fi:
* **Native Local Auto-Discovery:** Emulates local server discovery probes (UDP `7359` for Jellyfin, GDM UDP `32410–32414` for Plex, and mDNS/DNS-SD `_jellyfin._tcp`) so TV apps find the server automatically.
* **Zero Client Installs:** TVs, streaming boxes, iPads, and gaming consoles connect directly over standard local Wi-Fi without needing Tailscale installed.
* **Instant Wake from Standby:** Because client devices speak unencrypted LAN traffic to the local Tailbeacon gateway, connections never drop when the TV powers off.
* **Built-in Family Tech Support:** Dropping a Tailbeacon appliance (like a Raspberry Pi) at a family member's house gives them seamless access to your library, while giving you an encrypted Tailscale SSH / Subnet Router backdoor for remote tech support.

---

## 📐 Architecture

```text
 ┌────────────────────────────────────────────────────────┐
 │                   Offsite Mediaserver                  │
 │          (Plex, Jellyfin, Navidrome, Kavita)           │
 └───────────────────────────┬────────────────────────────┘
                             │
                      Encrypted Tailnet
                       (WireGuard/BBR)
                             │
                             ▼
 ┌────────────────────────────────────────────────────────┐
 │                 Tailbeacon Gateway                     │
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

## ✨ Features

- [x] **Jellyfin LAN Discovery:** Listens for broadcast probes (`Who is JellyfinServer?`) on UDP port `7359` and announces the local gateway.
- [x] **mDNS / DNS-SD Broadcasting:** Publishes `_jellyfin._tcp` zero-conf records for Swiftfin, Infuse, and modern TV clients.
- [x] **Universal Cross-Platform Binaries:** Pure Go (`CGO_ENABLED=0`) builds with zero external libc/musl runtime dependencies:
  - 📡 **OpenWrt & Travel Routers:** AArch64 (MediaTek Filogic, GL-MT6000), ARMv7 (GL-MT3000), MIPS/MIPSEL.
  - 🥧 **ARM Single-Board Computers:** Raspberry Pi Zero 2W / 3 / 4 / 5, Orange Pi, Rockchip RK3588.
  - ⚡ **Next-Gen RISC-V:** Native `linux/riscv64` (VisionFive 2, Milk-V, SpacemiT).
  - 💻 **x86_64:** Intel N100 mini-PCs, Proxmox, TrueNAS, unRAID, OPNsense/pfSense.
- [ ] **OpenWrt Native Service:** Lightweight `/etc/init.d/tailbeacon` procd service + UCI configuration (<4MB binary with UPX).
- [ ] **Plex GDM Relay:** Relays Plex "Good Day Mate" discovery packets across subnets.
- [ ] **Embedded Web GUI:** Modern, dark-mode configuration dashboard to toggle services, view active stream bitrates, and manage upstream routes.
- [ ] **Local Asset Caching:** Caches movie posters, episode thumbnails, and UI backdrops in local RAM/SSD to eliminate WAN round-trip latency on international links.

---

## ⚡ Quick Start

### Option A: Docker Compose

```yaml
services:
  tailbeacon:
    image: ghcr.io/shimco/tailbeacon:latest
    container_name: tailbeacon
    restart: unless-stopped
    network_mode: host
    environment:
      - UPSTREAM_JELLYFIN=http://mediaserver.horse-pancake.ts.net:8096
      - LOCAL_PORT=8096
      - SERVER_NAME="Living Room Mediaserver"
```

### Option B: Standalone Binary (Raspberry Pi / Linux)

```bash
# Download latest release
curl -sSL https://github.com/shimco/tailbeacon/releases/latest/download/tailbeacon-linux-arm64.tar.gz | tar -xz
sudo mv tailbeacon /usr/local/bin/

# Run as a background service
tailbeacon --upstream http://mediaserver:8096 --name "Home Gateway"
```

---

## 🗺️ Roadmap

- [ ] **v0.1:** Core Go streaming reverse proxy + UDP 7359 Jellyfin discovery responder.
- [ ] **v0.2:** Avahi / mDNS zero-conf integration and Plex GDM broadcast relay.
- [ ] **v0.3:** Web GUI dashboard for one-click service toggles and Tailnet health check.
- [ ] **v0.4:** LRU memory/disk cache for image thumbnails and static UI bundles.
- [ ] **v0.5:** Turnkey Raspberry Pi appliance installer script (`curl ... | bash`).

---

## 📄 License

Distributed under the [MIT License](LICENSE). Copyright (c) 2026 Graham Shimmin.
