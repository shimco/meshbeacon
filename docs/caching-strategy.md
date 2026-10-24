# Pragmatic Hardware & Asset Caching Strategy

Media server posters, photo thumbnails, book covers, and recipe photography are small (30KB–150KB), but requesting hundreds of them across a high-latency WAN mesh tunnel slows down Smart TV interfaces, e-readers, and kitchen tablets. 

Meshbeacon intercepts artwork queries (Jellyfin posters, Plex thumbnails, Immich photos, Audiobookshelf covers, Kavita/Komga book covers, Mealie recipes, and Navidrome album art), serving hits instantly from local memory and NVMe SSD.

---

## Hardware-Bracketed Memory Allocation

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

---

## Secondary Disk Tier (SSD, NVMe, Persistent Storage)

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
    "ttl": "auto"
  }
}
```

---

## Fail-Safe Protection and Auto-Sizing

* **Adaptive Free Space Scaling**: When `disk_size` is `"auto"`, Meshbeacon inspects available drive space via `statfs` and safely allocates up to 50% of free space (capped at **10 GB** on spacious NVMe drives for Immich photo frames, or down to **250 MB** on small USB thumb drives).
* **Extended Auto TTL**: When `ttl` is `"auto"`, Meshbeacon uses **30 days** in RAM-only mode, and automatically extends to **90 days** (`90d`) when persistent disk is enabled, preventing redundant WAN re-downloads of static posters and photo slideshows while allowing manual override (`"180d"`, `"1y"`).
* **50 MB Safety Floor & Self-Pruning**: If drive free space ever drops below 50 MB, or when 95% quota is reached, Meshbeacon automatically prunes the oldest 15% of assets down to 80%. If the drive is completely full, Meshbeacon gracefully falls back to live streaming with **zero crashes and zero TV playback interruptions**.
* **Video Stream Isolation**: Streaming chunks (`.ts`, `.m3u8`, Range requests) bypass the cache entirely and stream directly through the zero-allocation 32KB buffer pool (`sync.Pool`), ensuring video bitstreams never displace UI thumbnails.
