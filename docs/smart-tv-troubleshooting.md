# Smart TV Tips & Troubleshooting

This guide covers common network configurations, Wi-Fi router settings, and client app behaviors when connecting Smart TVs to Meshbeacon.

---

## 1. Static LAN IP / DHCP Reservation (Highly Recommended)

* While Meshbeacon dynamically adapts and broadcasts whatever LAN IP it currently holds, **Smart TV apps (LG webOS, Samsung Tizen, Android TV, Apple TV) cache the server's IP address locally once connected**.
* If your Raspberry Pi or host machine reboots and receives a different DHCP IP from your home router, the TV app will attempt to reconnect to the stale IP address, displaying a temporary *"Server Unreachable"* prompt until manually selected again.
* **The Fix**: Assign a **DHCP reservation** (or static IP) to your Raspberry Pi or host device in your home router's admin settings. This keeps the IP permanent across power cycles.

---

## 2. Wi-Fi Multicast & Router Settings

Meshbeacon uses UDP broadcast (port 7359 for Jellyfin, 32414 for Plex GDM) and Multicast DNS (`224.0.0.251:5353` for Bonjour/Apple TV). Some consumer Wi-Fi routers block multicast between Ethernet and Wi-Fi by default.

If your TV is on Wi-Fi and Meshbeacon is on an Ethernet-connected Raspberry Pi:
* **Wireless Multicast Forwarding (WMF)**: Ensure this is **Enabled** in your router's wireless settings.
* **IGMP Snooping**: Ensure this is **Enabled** so multicast discovery frames are forwarded to wireless clients.
* **AP Isolation / Client Isolation**: Ensure this is **Disabled**; otherwise, Wi-Fi devices cannot communicate with other local LAN devices.

> [!TIP]
> If a router aggressively blocks all multicast/broadcast discovery, you can always enter `http://<meshbeacon-lan-ip>:8096` directly into the TV app. Once entered, all streaming and artwork caching work immediately through the proxy.

---

## 3. Apple TV (Swiftfin) & iOS Permissions

Apple devices enforce strict privacy boundaries around local network discovery:
* On your Apple TV or iPhone/iPad, ensure that **Settings → Apps → Swiftfin → Local Network** is toggled **ON**.
* If disabled, tvOS will silently block Swiftfin from receiving mDNS announcements on `224.0.0.251`.

---

## 4. Google TV / Android TV / Fire TV

* On first launch, open the Jellyfin app to the server selection screen. Meshbeacon broadcasts its presence over the local network so your server appears on the screen automatically.
* If multiple network interfaces exist on the host running Meshbeacon (e.g. `eth0` and `wlan0`), you can pin Meshbeacon to a specific LAN IP using the `-advertise-host` flag:
  ```bash
  meshbeacon -advertise-host 192.168.0.2
  ```
