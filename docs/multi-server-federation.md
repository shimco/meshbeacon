# Multi-Server Proxying & Federation

This guide explains how to proxy multiple Jellyfin or Plex servers (such as your own home server and an offsite server) through a single Meshbeacon instance on your local network.

---

## Architecture: Sharing Connections

If both you and a friend run media servers and share libraries across Tailscale or WireGuard, Meshbeacon can expose both servers directly on your living room Wi-Fi:

```text
                           ┌──> [My NAS over Tailscale] (8096)
[LAN Smart TV] ──> Meshbeacon 
                           └──> [Friend's NAS over Tailscale] (8097)
```

---

## 1. Multi-Service Configuration (`meshbeacon.json`)

Assign each server its own local port on the LAN:

```json
{
  "advertise_addr": "auto",
  "services": [
    {
      "name": "jellyfin-home",
      "local_port": 8096,
      "upstream_url": "http://my-nas.tailnet:8096",
      "discovery": {
        "type": "jellyfin",
        "port": 7359
      }
    },
    {
      "name": "jellyfin-friend",
      "local_port": 8097,
      "upstream_url": "http://friends-nas.tailnet:8096"
    },
    {
      "name": "plex-home",
      "local_port": 32400,
      "upstream_url": "http://my-nas.tailnet:32400",
      "discovery": {
        "type": "plex",
        "port": 32414
      }
    },
    {
      "name": "plex-friend",
      "local_port": 32401,
      "upstream_url": "http://friends-nas.tailnet:32400"
    }
  ]
}
```

* **Independent Streaming**: Both streams are proxied independently through Meshbeacon's zero-allocation streaming engine.
* **Isolated Caching**: Artwork and thumbnails from each server are cached locally, accelerating browsing without either server's assets colliding.

---

## 2. How Client Discovery Works

### Apple TV, Swiftfin & Infuse (mDNS / Bonjour)
Multicast DNS natively supports multiple distinct instances per service type. Meshbeacon announces both:
* `Jellyfin Home._jellyfin._tcp.local` on port `8096`
* `Jellyfin Friend._jellyfin._tcp.local` on port `8097`

When you launch Swiftfin or Infuse, **both servers appear as separate discovery cards automatically**.

### Plex Account Federation
For Plex, you don't even need broadcast discovery. Plex has server federation built into its account system:
1. Your friend shares their Plex library with your Plex account.
2. In your friend's Plex Server Settings $\to$ **Network** $\to$ **Custom server access URLs**, they add your Meshbeacon address for their port:
   ```text
   http://<your-meshbeacon-lan-ip>:32401
   ```
3. When you open the Plex app on your TV, Plex pulls both servers from your account and connects to both at full local LAN speed through Meshbeacon.
