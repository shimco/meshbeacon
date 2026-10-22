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
- [x] **Plex GDM Discovery:** Responds to Plex "Good Day Mate" UDP discovery queries on port `32414`.
- [x] **mDNS / DNS-SD Broadcasting:** Publishes `_jellyfin._tcp` and `_plexmediasvr._tcp` zero-conf records for Apple TV (Swiftfin), Infuse, and iOS clients.
- [x] **Zero-Config Identity Auto-Detection:** Automatically queries upstream server endpoints (`/System/Info/Public` and `/identity`) to fetch Server IDs and names—no manual UUID configuration needed.
- [x] **Zero-Allocation Stream Proxy:** Hardened Go reverse proxy with pooled 32KB buffers (`sync.Pool`) for high-bitrate 4K streaming with minimal RAM usage.
- [x] **Docker Healthcheck & Self-Healing:** Built-in `-health-check` CLI verifying local proxy sockets and upstream reachability for Docker autoheal and Watchtower.
- [x] **Universal Cross-Platform Binaries:** Pure Go (`CGO_ENABLED=0`) builds with zero external libc/musl runtime dependencies:
  - 📡 **OpenWrt & Travel Routers:** AArch64 (MediaTek Filogic, GL-MT6000), ARMv7 (GL-MT3000), MIPS/MIPSEL.
  - 🥧 **ARM Single-Board Computers:** Raspberry Pi Zero 2W / 3 / 4 / 5, Orange Pi, Rockchip RK3588.
  - ⚡ **Next-Gen RISC-V:** Native `linux/riscv64` (VisionFive 2, Milk-V, SpacemiT).
  - 💻 **x86_64:** Intel N100 mini-PCs, Proxmox, TrueNAS, unRAID, OPNsense/pfSense.
- [ ] **OpenWrt Native Service:** Lightweight `/etc/init.d/tailbeacon` procd service + UCI configuration (<4MB binary with UPX).
- [ ] **Embedded Web GUI:** Modern, dark-mode configuration dashboard to toggle services, view active stream bitrates, and manage upstream routes.
- [ ] **Local Asset Caching:** Caches movie posters, episode thumbnails, and UI backdrops in local RAM/SSD to eliminate WAN round-trip latency on international links.

---

## 📋 Prerequisites

1. A device on your local home network to run Tailbeacon (Raspberry Pi, home server, mini-PC, NAS, or OpenWrt router).
2. **Tailscale running on that host machine** (or accessible via a local subnet router). Tailbeacon bridges the host's existing Tailscale connection into the unencrypted local LAN.

---

## ⚡ Quick Start

### Option A: Docker Compose (Recommended)

> [!NOTE]
> `network_mode: host` is **required** so Tailbeacon can bind and broadcast directly on the physical LAN's UDP discovery ports (`7359`, `32414`, `5353`).

1. Copy the example configuration:
   ```bash
   cp tailbeacon.example.json tailbeacon.json
   ```
2. Edit `tailbeacon.json` with your upstream Tailscale URLs (e.g. `http://mediaserver.your-tailnet.ts.net:8096`). Server IDs and names are auto-detected automatically if left blank:
   ```json
   {
     "advertise_addr": "auto",
     "services": [
       {
         "name": "jellyfin",
         "local_port": 8096,
         "upstream_url": "http://mediaserver.your-tailnet.ts.net:8096",
         "discovery": {
           "type": "jellyfin",
           "port": 7359
         }
       }
     ]
   }
   ```
3. Run with `docker compose`:
   ```bash
   docker compose up -d
   ```

### Option B: Standalone Binary (Raspberry Pi / Linux / OpenWrt)

Run directly using CLI flags or environment variables:

```bash
# Using CLI flags (Server IDs and names are automatically fetched from upstream)
tailbeacon -jellyfin http://mediaserver.your-tailnet.ts.net:8096 -plex http://mediaserver.your-tailnet.ts.net:32400

# Or via environment variables
export UPSTREAM_JELLYFIN="http://mediaserver.your-tailnet.ts.net:8096"
export UPSTREAM_PLEX="http://mediaserver.your-tailnet.ts.net:32400"
tailbeacon
```

---

## 💡 Smart TV Tips & Troubleshooting

* **Google TV / Android TV / Fire TV:**
  - On first launch, open the Jellyfin app to the server selection screen. Tailbeacon sends active broadcasts every 30s (with fast bursts on startup) to populate the server automatically.
* **Apple TV (Swiftfin) & iOS:**
  - tvOS and iOS require local network permission. Ensure **Settings → Apps → Swiftfin → Local Network** is toggled **ON**.
* **Wi-Fi Multicast / AP Isolation:**
  - If your TV is on Wi-Fi and Tailbeacon is on Ethernet, verify that **"Wireless Multicast Forwarding (WMF)"** or **"IGMP Snooping"** is enabled in your router settings, and **"AP Isolation"** is disabled.
  - If discovery is blocked by an aggressive router, entering `http://<tailbeacon-lan-ip>:8096` directly into the TV app always connects immediately through the streaming proxy.

---

## 🗺️ Roadmap

- [x] **v0.1:** Core Go streaming reverse proxy + zero-allocation `sync.Pool` buffer.
- [x] **v0.2:** Jellyfin UDP 7359 active + passive multi-subnet autodiscovery.
- [x] **v0.3:** Plex GDM UDP 32414 broadcast responder.
- [x] **v0.4:** Multicast DNS (mDNS / Bonjour) for Apple TV (Swiftfin) and Infuse.
- [x] **v0.5:** Automated Server ID & metadata detection from upstream servers.
- [x] **v0.6:** Built-in CLI health checking for Docker autoheal / Watchtower.
- [ ] **v0.7:** Turnkey Raspberry Pi appliance installer script (`curl ... | bash`).
- [ ] **v0.8:** LRU memory/disk cache for image thumbnails and static UI bundles.
- [ ] **v0.9:** Web GUI dashboard for one-click service toggles and Tailnet status.

---

## 📄 License

Distributed under the [MIT License](LICENSE). Copyright (c) 2026 Graham Shimmin.
