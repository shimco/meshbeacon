# Plex Direct Play & Bandwidth Throttling Resolution

This guide explains how to configure your Plex Media Server so Smart TV apps connect through Meshbeacon using unthrottled **Direct Play**, completely bypassing Plex's 2 Mbps Relay and avoiding unnecessary video transcoding.

---

## The Problem: Why Plex Relays Over VPNs

Unlike Jellyfin (which honors whatever local LAN IP sends a UDP discovery broadcast), **Plex clients consult `plex.tv` first**. 

When you launch Plex on an Apple TV, Roku, or Android TV:
1. The app queries `plex.tv` to fetch the authorized list of server connection addresses (`customConnections`).
2. If your remote server is only advertising its internal Tailscale or WireGuard IP (e.g. `100.64.0.5`), the TV app attempts to connect to that IP.
3. Because the TV has no VPN client installed and cannot route to `100.x.x.x`, the direct connection fails.
4. The Plex app falls back to the **Plex Relay** (`*.plex.direct` proxy service), capping the stream at **2 Mbps** and forcing your server's CPU or GPU to aggressively transcode 4K/1080p video down to 720p.

---

## Configuration Steps

To enable direct playback without transcoding through Meshbeacon:

### 1. Add Meshbeacon to Custom Access URLs
1. Open **Plex Web** on your server (`http://<your-plex-server>:32400/web`).
2. Navigate to **Settings** (wrench icon, top-right) $\to$ **Server** $\to$ **Network** (click **Show Advanced**).
3. In **Custom server access URLs**, add your Meshbeacon LAN gateway IP:
   ```text
   http://<meshbeacon-lan-ip>:32400
   ```
   *(If you also connect directly to your server over Tailscale/WireGuard on your laptop or phone, comma-separate both URLs so direct mesh connections continue to work on the road: `http://100.64.0.5:32400, http://192.168.0.2:32400`)*

### 2. Declare LAN Networks (The Magic Fix)
In **LAN Networks**, enter your home network range (and your mesh VPN subnet) using a broad subnet mask:
```text
192.168.0.0/255.255.0.0,100.0.0.0/255.0.0.0
```

#### Why This Step is Critical:
* By default, Plex treats any connection from an IP outside its immediate local subnet as **"Remote"**.
* Remote streams are subject to Plex's "Remote streaming quality" limits and default transcode rules.
* Entering `192.168.0.0/255.255.0.0` tells Plex that connections coming through Meshbeacon are **Local LAN traffic**, automatically granting unthrottled **Direct Play** by default.

### 3. Does This Disable Transcoding?
**No.** Transcoding still works whenever client hardware actually requires it. If an older TV cannot decode a specific video codec (e.g. AV1) or audio track (TrueHD / DTS), or if the client explicitly selects "Auto" quality, Plex will still transcode dynamically. However, it will do so based on actual client hardware capability and screen needs, rather than being forced to 2 Mbps by Plex Relay.

---

## How to Verify in the Playback HUD

1. Start playing a movie or show on your Smart TV or mobile app.
2. Open the playback menu $\to$ **Playback Info** (or **Stream Details**).
3. Look for:
   * **Player**: `Direct Play` (Original quality, zero transcode)
   * **Source**: `Local` / `Direct` (Streaming at full LAN speed through Meshbeacon)

If you see `Plex Relay` or `Transcoding to 720p 2 Mbps`, double-check that your Meshbeacon IP is saved under **Custom server access URLs** and that you restarted the client TV app.
