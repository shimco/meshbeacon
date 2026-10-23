# 🗼 Meshbeacon

> **Broadcast remote media servers across Tailscale, WireGuard, ZeroTier & Mesh networks into your local LAN with native Smart TV auto-discovery.**  
> *Stream Jellyfin, Plex, Immich, Audiobookshelf, and Mealie to Smart TVs, digital photo frames, kitchen tablets, and family devices with zero VPN client apps.*

[![CI](https://github.com/shimco/meshbeacon/actions/workflows/ci.yml/badge.svg)](https://github.com/shimco/meshbeacon/actions/workflows/ci.yml)
[![Docker](https://github.com/shimco/meshbeacon/actions/workflows/docker.yml/badge.svg)](https://github.com/shimco/meshbeacon/actions/workflows/docker.yml)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fshimco%2Fmeshbeacon-2496ED?logo=docker&logoColor=white)](https://github.com/shimco/meshbeacon/pkgs/container/meshbeacon)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shimco/meshbeacon)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Binary Size](https://img.shields.io/badge/Binary-<7MB-success.svg)](https://github.com/shimco/meshbeacon/releases)

> [!IMPORTANT]
> **Project Status: Public Beta (v0.8)**  
> Meshbeacon is actively being tested across diverse homelab setups. It has been validated in production with **Jellyfin on Docker** streaming to **Google TV and Android TV**.  
> Plex, Immich, Audiobookshelf, and OpenWrt travel router builds are currently in active community testing. **If you encounter discovery quirks with specific smart TVs or router setups, please open a [GitHub Issue](https://github.com/shimco/meshbeacon/issues)—your feedback directly drives the roadmap!**

---

## 🎯 Key Use Cases

| Scenario | How Meshbeacon Solves It |
| :--- | :--- |
| **🏠 Offsite Homelab to Living Room TV** | You host Jellyfin/Plex on an offsite server, Hetzner VPS, or secondary homelab connected via WireGuard/Tailscale. Running Meshbeacon on any local device makes the server appear as a native local device on your LAN Wi-Fi—no VPN apps needed on your TVs. |
| **👵 Parents & Family Homes ("Drop a Pi")** | Non-technical parents or friends struggle with VPN logins, standby disconnects, and mobile apps. Give them a Raspberry Pi running Meshbeacon: plug it into their router, and their Smart TVs and iPads discover your server automatically without failing SFA benchmarks. |
| **🖼️ Immich & Digital Photo Frames** | Wall-mounted tablets, smart displays, and Raspberry Pi digital photo frames (Immich Kiosk / Immich Frame) stay awake and stream high-res family photo albums across your mesh without VPN drops. Mobile phones on local Wi-Fi auto-discover the server via mDNS (`_immich._tcp`) for silent background backups. |
| **🍳 Mealie & Kitchen Tablets** | Running a shared recipe server like Mealie offsite? Kitchen tablets and family phones browse and scale dinner recipes smoothly on local Wi-Fi without needing a VPN tunnel switched on. |
| **🛡️ Smart TV VLAN Isolation** | You isolate Smart TVs on an untrusted **IoT VLAN**, while your media server sits on a private **Server VLAN**. Because discovery broadcasts cannot cross VLANs, the TV cannot find the server. Meshbeacon runs on the IoT VLAN to safely proxy streams and broadcast discovery locally. |
| **🧳 Travel Routers & Airbnbs** | Running Meshbeacon on a GL.iNet travel router or mini-PC connects back to your home mesh network. Any phone, laptop, or Chromecast connecting to your travel Wi-Fi immediately sees your home media library. |
| **🌐 Any Mesh / Overlay Network** | **100% network-agnostic.** Works out-of-the-box with **Tailscale, WireGuard, ZeroTier, Nebula, Headscale, Netbird**, or standard internal routed subnets. |

---

## 💡 The Problem with VPNs on Smart TVs

Running VPN client apps directly on Smart TVs (Google TV, Android TV, Apple TV, Fire TV, Roku, webOS, Tizen) causes chronic friction:
1. **Aggressive Standby Sleep:** Smart TV OSes ruthlessly kill background VPN processes to conserve RAM. Waking the TV from standby leaves the WireGuard tunnel dead, causing media apps to buffer or hang indefinitely.
2. **Family Support Overhead:** Explaining VPN client toggles, MFA, or IP configuration to non-technical partners or parents is frustrating, and doesn't pass SAF benchmarks...
3. **Underpowered Hardware:** Low-cost TV chipsets struggle with background cryptographic packet processing during high-bitrate 4K HDR streams, and they crash.

## 🚀 The Solution: Meshbeacon

**Meshbeacon** is a lightweight local LAN gateway and discovery beacon. You run it on a single machine on your local network (a Raspberry Pi, mini-PC, NAS, home server, or OpenWrt router). 

Meshbeacon bridges your remote media server into your local home Wi-Fi:
* **Native Local Auto-Discovery:** Emulates local server discovery probes (UDP `7359` for Jellyfin, GDM UDP `32414` for Plex, and mDNS/DNS-SD `_jellyfin._tcp`) so TV apps find the server automatically.
* **Zero Client Installs:** TVs, streaming boxes, iPads, and gaming consoles connect directly over standard local Wi-Fi without needing VPN apps.
* **Instant Wake from Standby:** Because client devices speak unencrypted LAN traffic to the local Meshbeacon gateway, connections never drop when the TV powers off.
* **Zero-Allocation 4K Streaming:** Built-in hardened reverse proxy using a 32KB buffer pool (`sync.Pool`) delivers high-throughput video streaming with near-zero CPU and <10MB of RAM.

---

## 📐 Architecture

```text
 ┌────────────────────────────────────────────────────────┐
 │              Offsite Media Server / NAS                │
 │          (Plex, Jellyfin, Navidrome, Kavita)           │
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

## ✨ Features

- [x] **Jellyfin LAN Discovery:** Listens for broadcast probes (`Who is JellyfinServer?`) on UDP port `7359` and actively announces presence across subnets.
- [x] **Plex GDM Discovery:** Responds to Plex "Good Day Mate" UDP discovery queries on port `32414`.
- [x] **mDNS / DNS-SD Broadcasting:** Publishes `_jellyfin._tcp` and `_plexmediasvr._tcp` zero-conf records for Apple TV (Swiftfin), Infuse, and iOS clients.
- [x] **Zero-Config Identity Auto-Detection:** Automatically queries upstream server endpoints (`/System/Info/Public` and `/identity`) to fetch Server IDs and names—no manual UUID configuration needed.
- [x] **Zero-Allocation Stream Proxy:** Hardened Go reverse proxy with pooled 32KB buffers (`sync.Pool`) for high-bitrate 4K streaming with minimal RAM usage.
- [x] **Docker Healthcheck & Self-Healing:** Built-in `-health-check` CLI verifying local proxy sockets and upstream reachability for Docker Autoheal and Watchtower.
- [x] **Ultra-Lightweight & Efficient:** Consumes only **~6MB of RAM** and **<0.2% CPU** during active playback.
- [x] **Universal Cross-Platform Binaries:** Pure Go (`CGO_ENABLED=0`) builds with zero external libc/musl runtime dependencies:
  - 📡 **OpenWrt & Travel Routers:** AArch64 (MediaTek Filogic, GL-MT6000), ARMv7 (GL-MT3000), MIPS/MIPSEL.
  - 🥧 **ARM Single-Board Computers:** Raspberry Pi Zero 2W / 3 / 4 / 5, Orange Pi, Rockchip RK3588.
  - ⚡ **Next-Gen RISC-V:** Native `linux/riscv64` (VisionFive 2, Milk-V, SpacemiT).
  - 💻 **x86_64:** Intel N100 mini-PCs, Proxmox, TrueNAS, unRAID, OPNsense/pfSense.
- [x] **Dual-Tier Asset Caching:** High-speed in-memory LRU cache with hardware-bracketed auto-scaling (8MB on 128MB routers to 512MB on servers) to eliminate SD card/flash wear, plus optional persistent SSD/disk tier for multi-gigabyte library caching.
- [x] **Generic mDNS & Multi-App Discovery:** Extensible DNS-SD engine supporting Immich photo sync (`_immich._tcp`), AirPrint / CUPS (`_ipp._tcp`), HomeKit (`_hap._tcp`), and custom services without extra code or dependencies.
- [x] **OpenWrt Native Service & Turnkey Installer:** Lightweight procd `/etc/init.d/meshbeacon` service and POSIX install/uninstall script with SHA-256 verification (<3MB binary).
- [ ] **Embedded Web GUI:** Modern, dark-mode configuration dashboard to toggle services, view active stream bitrates, and manage upstream routes.

---

## 📋 Prerequisites

1. A device on your local home network to run Meshbeacon (Raspberry Pi, home server, mini-PC, NAS, or OpenWrt router).
2. **Network connectivity to your remote media server** via Tailscale, WireGuard, ZeroTier, Nebula, or a direct IP route. Meshbeacon bridges that existing connection into the unencrypted local LAN.

---

## ⚡ Quick Start

### Option A: Docker Compose & Container (Recommended)

> [!NOTE]
> `network_mode: host` is **required** so Meshbeacon can bind and broadcast directly on the physical LAN's UDP discovery ports (`7359`, `32414`, `5353`).
> 
> **🧠 Multi-Arch Auto-Detection:** The official image (`ghcr.io/shimco/meshbeacon:latest`) includes multi-platform manifests for `linux/amd64` (Intel/AMD), `linux/arm64` (Raspberry Pi 4/5, Apple Silicon), and `linux/arm/v7` (Raspberry Pi 2/3/Zero 2W). Docker queries and pulls the correct architecture automatically.

#### 1-Line Quickstart (Zero-Config Test)
To test immediately without creating files:
```bash
docker run -d --name meshbeacon --restart=unless-stopped --net=host \
  -e UPSTREAM_JELLYFIN="http://media-server.your-tailnet.ts.net:8096" \
  ghcr.io/shimco/meshbeacon:latest
```

#### Production with Docker Compose
1. Create or copy the example configuration:
   ```bash
   cp meshbeacon.example.json meshbeacon.json
   ```
2. Edit `meshbeacon.json` with your upstream URLs (e.g. Tailscale MagicDNS, WireGuard IP, or local domain). Server IDs and names are auto-detected automatically if left blank:
   ```json
   {
     "advertise_addr": "auto",
     "services": [
       {
         "name": "jellyfin",
         "local_port": 8096,
         "upstream_url": "http://media-server.your-tailnet.ts.net:8096",
         "discovery": {
           "type": "jellyfin",
           "port": 7359
         }
       },
       {
         "name": "plex",
         "local_port": 32400,
         "upstream_url": "http://media-server.your-tailnet.ts.net:32400",
         "discovery": {
           "type": "plex",
           "port": 32414
         }
       }
     ]
   }
   ```
3. Use the included `docker-compose.yml`:
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
4. Start the container:
   ```bash
   docker compose up -d
   ```

### Option B: Homebrew (macOS & Linux)

Install via the official Shimco tap:

```bash
# Add the tap and install
brew tap shimco/tap
brew install meshbeacon

# Optional: Run as a background daemon (auto-restarts on login)
brew services start meshbeacon
```

Configuration is stored at `$(brew --prefix)/etc/meshbeacon/meshbeacon.json`.

---

### Option C: Debian / Ubuntu Package (`.deb`)

For Ubuntu Server, Debian, Raspberry Pi OS, Linux Mint, or Pop!_OS:

```bash
# Download the latest deb for your architecture (amd64, arm64, or armhf)
curl -LO https://github.com/shimco/meshbeacon/releases/latest/download/meshbeacon_linux_amd64.deb

# Install package (automatically configures and starts the systemd service)
sudo apt install ./meshbeacon_linux_amd64.deb
```

Edit your upstream media server URLs at `/etc/meshbeacon/meshbeacon.json`, then restart:
```bash
sudo systemctl restart meshbeacon
```

---

### Option D: The YOLO One-Liner (curl | sh)

For when you just want it running *right now*. Automatically detects your CPU architecture (x86_64, ARM64, ARMv7, MIPS), verifies SHA-256 checksums against official releases, drops in the binary, and wires up either **OpenWrt procd** or **Linux systemd**:

```bash
# Install or upgrade to latest release
curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh

# Clean uninstallation (removes service and binary, preserves configuration)
curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh -s -- --uninstall

# Full purge (removes service, binary, and /etc/meshbeacon configuration)
curl -sSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh -s -- --purge
```

#### Manual Run / CLI Flags
You can also run directly from the terminal without a background daemon:

```bash
# Using CLI flags (Server IDs and names are automatically fetched from upstream)
meshbeacon -jellyfin http://media-server.your-tailnet.ts.net:8096 -plex http://mediaserver.your-tailnet.ts.net:32400

# Or via environment variables
export UPSTREAM_JELLYFIN="http://media-server.your-tailnet.ts.net:8096"
export UPSTREAM_PLEX="http://media-server.your-tailnet.ts.net:32400"
meshbeacon
```

---

## 🧪 Hardware Compatibility & Tested Environments

Meshbeacon is written in pure Go with zero external runtime dependencies (`CGO_ENABLED=0`). We believe in being transparent about which environments have been physically validated versus those currently in community testing:

| Environment | Hardware / OS | Architecture | Status | Validation Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Linux Server** | Ubuntu Server 24.04 LTS (`homeserver`) | `amd64` (x86_64) | 🟢 **Verified** | Bare-metal (`systemd`), APT package (`.deb`), and Docker Compose |
| **Apple Mac** | macOS 15 Sequoia (`themac`) | `arm64` (Apple Silicon) | 🟢 **Verified** | Standalone binary and Homebrew Tap (`brew services` background daemon) |
| **Docker / Compose** | Docker 26+ / Compose 2.26+ | `amd64`, `arm64` | 🟢 **Verified** | Official multi-arch GHCR image with self-healing healthchecks |
| **Raspberry Pi** | Raspberry Pi 4 (Debian 12 / RPi OS) | `arm64` | 🟡 **Active Testing** | Tested over Tailscale mesh network |
| **OpenWrt Router** | MediaTek Filogic / GL.iNet (Beryl AX / Flint) | `arm64` / `mips` | 🟡 **Community Testing** | POSIX procd service installer tested; seeking community test reports |
| **TrueNAS / BSD** | FreeBSD / TrueNAS SCALE | `amd64` | ⚪ **Planned** | Community feedback welcomed |

> [!TIP]
> **Help Validate Other Hardware!**  
> If you are running Meshbeacon on an OpenWrt router, Orange Pi, Rockchip SBC, Proxmox LXC, or an older Raspberry Pi, please [open a Discussion](https://github.com/shimco/meshbeacon/discussions) or Issue with your test notes—we'd love to add your hardware setup to the verified list!

---

## 💡 Smart TV Tips & Troubleshooting

* **Google TV / Android TV / Fire TV:**
  - On first launch, open the Jellyfin app to the server selection screen. Meshbeacon sends active broadcasts every 30s (with fast bursts on startup) to populate the server automatically.
* **Apple TV (Swiftfin) & iOS:**
  - tvOS and iOS require local network permission. Ensure **Settings → Apps → Swiftfin → Local Network** is toggled **ON**.
* **Static LAN IP / DHCP Reservation (Recommended for Family Setups):**
  - While Meshbeacon dynamically adapts and broadcasts whatever LAN IP it currently holds, **Smart TV apps (LG webOS, Samsung Tizen, Android TV, Apple TV) cache the server's IP address locally once connected**.
  - If the Raspberry Pi or host machine reboots and receives a different DHCP IP from the router, the TV app will attempt to reconnect to the stale IP address, displaying a temporary "Server Unreachable" prompt until manually selected again.
  - **Frictionless Fix:** Assign a **DHCP reservation** (or static IP) to your Raspberry Pi or host device in your home router's admin settings. This reduces support nagging and should keep the connection stable.
* **Wi-Fi Multicast / AP Isolation:**
  - If your TV is on Wi-Fi and Meshbeacon is on Ethernet, verify that **"Wireless Multicast Forwarding (WMF)"** or **"IGMP Snooping"** is enabled in your router settings, and **"AP Isolation"** is disabled.
  - If discovery is blocked by an aggressive router, entering `http://<meshbeacon-lan-ip>:8096` directly into the TV app always connects immediately through the streaming proxy.

---

## 💾 Pragmatic Hardware & Asset Caching Strategy
 
Media server posters and photo thumbnails are small (30KB–150KB), but requesting hundreds of them across a high-latency WAN mesh tunnel slows down Smart TV interfaces. Meshbeacon intercepts artwork queries (`/Items/{id}/Images/*` for Jellyfin and `/photo/:/transcode` for Plex), serving hits instantly from local memory.
 
### Hardware-Bracketed "Auto" Memory Allocation
 
Rather than a flat percentage that starves micro-routers or wastes gigabytes on home servers, Meshbeacon auto-detects total system RAM on startup and selects a sensible hardware-tailored bracket:
 
| Total System RAM | Target Hardware Profile | Auto RAM Cache | Approx Capacity (~40KB/poster) | User Experience |
| :--- | :--- | :--- | :--- | :--- |
| **$\le$ 128 MB** | GL.iNet Mango / micro-routers | **8 MB** | ~200 posters | Instant home rows (Continue Watching / Next Up) without starving router OS |
| **$\le$ 512 MB** | Pi Zero 2W / Beryl AX | **32 MB** | ~800 posters | Fluid scrolling through active TV seasons |
| **$\le$ 1 GB** | Raspberry Pi 3 / basic SBCs | **96 MB** | ~2,500 posters | Entire typical movie/show library cached in RAM |
| **$\le$ 2 GB** | Raspberry Pi 4 (2GB) | **192 MB** | ~4,800 posters | Full libraries + high-res episode thumbs |
| **$\le$ 4 GB** | Pi 4 / 5 (4GB) | **256 MB** | ~6,500 posters | Deep libraries with actor portraits and fanart |
| **$\gt$ 4 GB** | Homelabs / Mini-PCs / Workstations | **512 MB** | ~13,000+ posters | Zero disk/mesh queries ever during browsing |
 
> [!TIP]
> **Zero Flash & SD-Card Wear:** By default, caching operates 100% in-memory (`RAM LRU`). Meshbeacon **never** writes temporary cache files to the internal filesystem on OpenWrt routers (protecting 16MB SPI flash from filling up) or Raspberry Pis (protecting MicroSD cards from write wear).
 
### Secondary Disk Tier (SSD / NVMe / Persistent Disk)

If you run Meshbeacon on a homelab server, mini-PC, or NAS with an SSD or NVMe drive, configure `disk_dir` to store gigabytes of artwork permanently across reboots:

```bash
# 1. Balanced Mode (Default "auto" when disk is enabled):
# Automatically scales RAM to a 64MB–128MB hot-buffer, auto-sizes disk up to 10GB, and bumps TTL to 90 days
meshbeacon -cache-dir /mnt/nvme/meshbeacon

# 2. Pure NVMe Disk Only (0 MB RAM used for images):
# Reads 100% of cached assets directly from NVMe, dropping base RAM to ~4MB
meshbeacon -cache-ram off -cache-dir /mnt/nvme/meshbeacon -cache-disk-size 2GB

# 3. Privacy & Maintenance: Clear disk cache anytime
meshbeacon -clear-cache
```

Or in `meshbeacon.json`:
```json
{
  "cache": {
    "enabled": true,
    "ram_size": "auto",
    "disk_dir": "/var/cache/meshbeacon",
    "disk_size": "auto",
    "ttl": "90d"
  }
}
```

> [!NOTE]
> **Intelligent "Auto" Sizing & Fail-Safe Protection:**
> * **Adaptive Free Space Scaling**: When `disk_size` is `"auto"`, Meshbeacon inspects available drive space via `statfs` and safely allocates up to 50% of free space (capped at **10 GB** on spacious NVMe drives for Immich photo frames, or down to **250 MB** on small USB thumb drives).
> * **Extended 90-Day TTL**: Disk cache automatically extends default TTL to **90 days** (`90d`), preventing redundant WAN re-downloads of static posters and photo slideshows.
> * **50 MB Safety Floor & Self-Pruning**: If drive free space ever drops below 50 MB, or when 95% quota is reached, Meshbeacon automatically prunes the oldest 15% of assets down to 80%. If the drive is completely full, Meshbeacon gracefully falls back to live streaming with **zero crashes and zero TV playback interruptions**.
> * Set `ram_size: "off"` for **Pure Disk Mode** if you want zero image data stored in RAM.

*(Video streaming chunks `.ts`, `.m3u8`, and Range requests bypass the cache entirely and stream directly through the zero-allocation 32KB buffer pool).*
 
---
 
## 🗺️ Roadmap
 
- [x] **v0.1:** Core Go streaming reverse proxy + zero-allocation `sync.Pool` buffer.
- [x] **v0.2:** Jellyfin UDP 7359 active + passive multi-subnet autodiscovery.
- [x] **v0.3:** Plex GDM UDP 32414 broadcast responder.
- [x] **v0.4:** Multicast DNS (mDNS / Bonjour) for Apple TV (Swiftfin) and Infuse.
- [x] **v0.5:** Automated Server ID & metadata detection from upstream servers.
- [x] **v0.6:** Built-in CLI health checking for Docker autoheal / Watchtower.
- [x] **v0.7:** Hardware-aware Dual-Tier Asset Caching (RAM LRU vs. persistent disk).
- [x] **v0.8:** Generic mDNS profiles & Immich photo sync (`_immich._tcp`, `_ipp._tcp`, custom DNS-SD).
- [ ] **v0.9:** Embedded Web GUI with dual-interface binding (local LAN + remote Mesh).
- [ ] **v1.0:** Turnkey Raspberry Pi appliance installer script & OpenWrt procd package.
 
---
 
## 📄 License
 
Distributed under the [MIT License](LICENSE). Copyright (c) 2026 Graham Shimmin.

