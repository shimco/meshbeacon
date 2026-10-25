# Getting Started with Meshbeacon

This guide covers installing and configuring Meshbeacon on your local network.

---

## Installation Options

### Option A: Docker Compose (Recommended for Home Servers & NAS)

1. Create or copy a configuration file `meshbeacon.json`:
   ```bash
   cp meshbeacon.example.json meshbeacon.json
   ```

2. Point `upstream_url` to your remote server over Tailscale, WireGuard, or local network:
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

3. Run with `docker-compose.yml`:
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
         # Optional: Persist multi-gigabyte artwork cache across restarts
         # - /mnt/nvme/meshbeacon-cache:/var/cache/meshbeacon
   ```

   > [!IMPORTANT]
   > `network_mode: host` is **required** so Meshbeacon can bind directly to the host's physical network interface to broadcast UDP packets (`7359`, `32414`) and Multicast DNS (`224.0.0.251:5353`) across your home Wi-Fi.

4. Start the container:
   ```bash
   docker compose up -d
   ```

---

### Option B: Homebrew (macOS & Linux)

For Mac mini home servers, MacBooks, or Linux systems running Homebrew:

```bash
# Add the official tap and install
brew tap shimco/tap
brew install meshbeacon

# Optional: Run as a background daemon (auto-restarts on login / boot)
brew services start meshbeacon
```

* **Configuration Path**: `$(brew --prefix)/etc/meshbeacon/meshbeacon.json`
* **Logs**: `$(brew --prefix)/var/log/meshbeacon.log`

---

### Option C: Debian / Ubuntu Package (`.deb`)

For Ubuntu Server, Debian 12, Raspberry Pi OS (Debian bookworm), Linux Mint, or Pop!_OS:

```bash
# 1. Download the latest .deb for your CPU architecture (amd64, arm64, armv7)
ARCH=$(dpkg --print-architecture)
TAG=$(curl -s https://api.github.com/repos/shimco/meshbeacon/releases/latest | grep tag_name | cut -d '"' -f 4)
curl -LO "https://github.com/shimco/meshbeacon/releases/download/${TAG}/meshbeacon_linux_${ARCH}.deb"

# 2. Install the package
sudo dpkg -i "meshbeacon_linux_${ARCH}.deb"

# 3. Edit the configuration
sudo nano /etc/meshbeacon/meshbeacon.json

# 4. Enable and start the systemd service
sudo systemctl enable --now meshbeacon
```

---

### Option D: The "YOLO" One-Liner Install (Raspberry Pi & Generic Linux)

For quick automated setup on Raspberry Pi OS, Alpine, or standard Linux distros:

```bash
curl -fsSL https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sudo bash
```

This downloads the correct static binary for your architecture, installs it to `/usr/local/bin/meshbeacon`, creates a template configuration in `/etc/meshbeacon/meshbeacon.json`, and registers a systemd service with automated restart resilience.

---

### Option E: OpenWrt Travel Routers (GL.iNet, MediaTek Filogic)

Run Meshbeacon directly on your travel router or home gateway:

```sh
# Run the automated installer directly on the router via SSH
wget -O - https://raw.githubusercontent.com/shimco/meshbeacon/main/install.sh | sh
```

The installer:
1. Installs the static multi-arch binary to `/usr/bin/meshbeacon`.
2. Registers a native OpenWrt `procd` service at `/etc/init.d/meshbeacon` with respawn protection and enables it on boot.
3. Automatically installs a LuCI web menu item under **Services -> Meshbeacon** (linking directly to the embedded dashboard on port 7350).
4. Automatically starts the service.

On initial boot, if upstream media servers have not been configured yet, Meshbeacon enters **Standby Mode** and serves a setup wizard at `http://<router-ip>:7350` or `http://meshbeacon.local:7350`.

---

## 🖥️ Web Dashboard & Standby Setup Mode

Meshbeacon includes a zero-dependency, embedded web dashboard running on port `7350` (`http://meshbeacon.local:7350`):

* **Standby Mode (First-Run Wizard)**: If started with an unconfigured or template configuration, Meshbeacon holds proxy traffic dormant and displays a prominent setup banner and web form to configure your upstream server in seconds.
* **Active Mode**: Live service status, real-time cache hit ratios, bandwidth metrics, and update notifications.
* **Config Editor**: Edit and persist configuration directly from the browser with automatic atomic reload.

---

## 🛡️ Configuration Validation & Auto-Updates

### Pre-flight Config Testing
Test configuration syntax and port availability before deploying:
```bash
meshbeacon -test-config -config /etc/meshbeacon/meshbeacon.json
```

### Checking for Updates
```bash
# Check if a newer version is available on GitHub
meshbeacon -check-update

# Automatically download, verify SHA-256, test config, and safely update standalone binary
meshbeacon -update
```

*(Note: On Docker and package-managed installs, Meshbeacon detects its containerized environment and prompts you to update via `docker compose pull` or `apt`.)*

> [!NOTE]
> **Privacy & Network Transparency:**
> Update checks query the public GitHub Releases API (`api.github.com/repos/shimco/meshbeacon/releases/latest`) anonymously over standard HTTPS with zero tokens or tracking telemetry. Automatic daily checks are disabled by default (`"auto_update": false`), and Meshbeacon runs 100% offline in air-gapped homelabs.

---

## ⚙️ Configuration Reference (`meshbeacon.json`)

```json
{
  "schema_version": 1,
  "admin_port": 7350,
  "advertise_addr": "auto",
  "cache": {
    "enabled": true,
    "ram_size": "auto",
    "disk_dir": "",
    "disk_size": "auto",
    "ttl": "auto"
  },
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
    },
    {
      "name": "jellyseerr",
      "local_port": 5055,
      "upstream_url": "http://media-server.your-tailnet.ts.net:5055"
    }
  ]
}
```

### Key Settings:
* **`advertise_addr`**: Set to `"auto"` to automatically discover and broadcast your primary LAN IP. Or specify a static IP (e.g., `"192.168.0.2"`).
* **`cache.ram_size`**: `"auto"` (scales dynamically based on system RAM), `"64MB"`, or `"off"` for pure disk mode.
* **`cache.disk_dir`**: Leave blank `""` for 100% in-memory caching (zero SD card wear). Set to an SSD/NVMe path (e.g. `"/var/cache/meshbeacon"`) for persistent storage.
* **`cache.ttl`**: `"auto"` (30 days in RAM mode, 90 days in persistent disk mode), or explicit duration like `"180d"`.

---

## Command-Line Flags and Environment Variables

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-config <path>` | `CONFIG_FILE` | auto | Path to `meshbeacon.json` (auto-checks `/etc/meshbeacon/meshbeacon.json`) |
| `-advertise-host <ip>` | `ADVERTISE_HOST` | `auto` | LAN IP advertised to client apps |
| `-cache-ram <size>` | `CACHE_RAM` | `auto` | Max RAM for artwork cache (`auto`, `64MB`, `off`) |
| `-cache-dir <path>` | `CACHE_DIR` | `""` | Directory for persistent NVMe/SSD disk cache |
| `-cache-disk-size <size>` | `CACHE_DISK_SIZE` | `auto` | Max disk space for persistent cache (`auto`, `2GB`) |
| `-access-log` | - | `false` | Log every HTTP request to stdout (keep false to prevent SD card wear) |
| `-clear-cache` | - | - | Delete all cached assets from disk and exit |
| `-health-check` | - | - | Verify local listeners and upstream reachability, then exit |
| `-version` | - | - | Print Meshbeacon version and exit |
