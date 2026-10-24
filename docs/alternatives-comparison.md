# Why Meshbeacon? Comparing Architectural Alternatives

I built Meshbeacon to solve my own problem. Getting smart TVs and tablets to work at family members' homes with Plex or Jellyfin is a pain. Expecting a non-technical family member to discover and connect to an offsite media server without having them join a VPN is an exercise in frustration, and there is only so much explaining what a VPN does that one man can do. Even with Tailscale installed on their TVs, the connection frequently dropped because Android TV ruthlessly kills off background processes. I wanted a simple, high-SAF solution that runs silently in the background and doesn't require my relatives to do anything other than plug it in and connect to Wi-Fi.

When looking at this challenge, several architectural alternatives exist. This document breaks down why Meshbeacon was designed specifically for living room setups, and how it compares to traditional caching proxies, VPN subnet routers, and local DNS servers.

---

## 1. Why not run a Squid cache on the Pi?

Squid is a venerable forward proxy, but it is fundamentally mismatched for smart home media setups:

* **Smart TVs Have No Proxy Settings**:
  LG webOS, Samsung Tizen, and Roku devices provide no HTTP proxy settings in their network configuration. Even when present on mobile OSes, media apps like Plex and Jellyfin ignore OS proxy settings. Meshbeacon is a dedicated **reverse proxy** that binds directly to standard service ports (`:8096`, `:32400`, `:5055`), making proxying completely transparent to client devices.
* **Zero Discovery Capability**:
  Squid cannot listen on or broadcast UDP `7359` (Jellyfin), UDP `32414` (Plex GDM), or RFC 6762 mDNS (`224.0.0.251:5353`). Your TV screen simply remains on *"Server Offline"*.
* **Video Stream Cache Pollution**:
  Without complex custom rules, Squid attempts to cache multi-gigabyte video bitstreams, thrashing storage and evicting UI thumbnails. Meshbeacon explicitly isolates static artwork (posters, backdrops, episode thumbs with 90-day auto-TTL) while streaming video chunks through zero-allocation pooled memory buffers (`sync.Pool`) without touching the cache.
* **Flash Storage & SD Card Wear**:
  Squid continuously writes swap indexes and access logs to disk, quickly destroying MicroSD cards on Raspberry Pis and flash memory on OpenWrt routers. Meshbeacon defaults to RAM-first caching and in-memory telemetry with zero disk access logging.

---

## 2. Why not use Tailscale Services or Subnet Routers?

Tailscale is fantastic for mesh networking, but Layer-3/4 packet routing fails the **"Living Room TV Test"**:

* **Smart TVs Cannot Join Tailscale**:
  Most consumer Smart TVs (Samsung Tizen, LG webOS, Roku) do not support running Tailscale nodes or authenticating into a tailnet.
* **Broadcast and Multicast Do Not Cross the Mesh**:
  Link-local mDNS (`224.0.0.251`) and UDP broadcasts (`255.255.255.255`) cannot traverse Tailscale tunnels or subnet routers. Without a local beacon shouting on the living room subnet, auto-discovery is completely broken.
* **Plex Relay 2 Mbps Throttling**:
  Even if a TV could route packets to an offsite Tailscale IP, Plex clients fall back to the throttled 2 Mbps Plex Relay unless local GDM probes answer on the LAN and the subnet is declared local.
* **Zero Layer-7 Acceleration**:
  Tailscale does not cache. Every poster and menu click has to round-trip across the WAN mesh link every single time. Meshbeacon serves cached UI assets locally in <2ms.

---

## 3. Pairing with AdGuard Home or Pi-hole

Meshbeacon deliberately does not include a full DNS server to avoid unnecessary scope creep. Instead, running **AdGuard Home (or Pi-hole) alongside Meshbeacon** on the same Raspberry Pi creates a complete, turnkey local appliance:

* **AdGuard Home** manages port 53: network-wide ad blocking and custom `.lan` / split-horizon rewrites (`jellyfin.lan`, `requests.lan`).
* **Meshbeacon** manages media ports: reverse proxying over Tailscale, dual-tier caching, and mDNS / UDP living room discovery.

### A Note on `.local` vs `.lan`:
* **`.local`** is reserved exclusively by **RFC 6762** for Multicast DNS (mDNS). Meshbeacon automatically announces `<hostname>.local` without requiring any DNS server.
* **`.lan`** (or `.home.arpa`) is for centralized unicast DNS servers like AdGuard Home or dnsmasq. 
* Never set your router's domain search suffix to `.local`, as macOS and iOS treat `.local` strictly as mDNS and will hang trying to resolve unicast queries.
