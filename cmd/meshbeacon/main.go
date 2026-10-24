package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/shimco/meshbeacon/internal/cache"
	"github.com/shimco/meshbeacon/internal/config"
	"github.com/shimco/meshbeacon/internal/discovery"
	"github.com/shimco/meshbeacon/internal/health"
	"github.com/shimco/meshbeacon/internal/netutil"
	"github.com/shimco/meshbeacon/internal/proxy"
)

var version = "v0.8.0"

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func main() {
	var (
		configFile       = flag.String("config", getEnv("CONFIG_FILE", ""), "Path to JSON configuration file")
		advertiseHost    = flag.String("advertise-host", getEnv("ADVERTISE_HOST", "auto"), "Local host/IP advertised to LAN clients ('auto' to detect)")
		upstreamJellyfin = flag.String("jellyfin", getEnv("UPSTREAM_JELLYFIN", ""), "Upstream Jellyfin URL (e.g. http://mediaserver:8096)")
		upstreamPlex     = flag.String("plex", getEnv("UPSTREAM_PLEX", ""), "Upstream Plex URL (e.g. http://mediaserver:32400)")
		upstreamImmich   = flag.String("immich", getEnv("UPSTREAM_IMMICH", ""), "Upstream Immich URL (e.g. http://mediaserver:2283)")
		cacheRAM         = flag.String("cache-ram", getEnv("CACHE_RAM", "auto"), "Asset cache RAM size ('auto', '64MB', 'off')")
		cacheDir         = flag.String("cache-dir", getEnv("CACHE_DIR", ""), "Optional persistent disk cache directory (default off)")
		cacheDisk        = flag.String("cache-disk-size", getEnv("CACHE_DISK_SIZE", "auto"), "Max disk cache size if cache-dir is enabled ('auto', '2GB')")
		healthCheck      = flag.Bool("health-check", false, "Run health check against configured services and exit")
		clearCache       = flag.Bool("clear-cache", false, "Clear all cached assets from disk and exit")
		showVersion      = flag.Bool("version", false, "Print Meshbeacon version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("meshbeacon %s\n", version)
		os.Exit(0)
	}

	// Cache Clearing Mode (Used for manual maintenance or during uninstall/purge)
	if *clearCache {
		targetDir := *cacheDir
		if targetDir == "" && *configFile != "" {
			if loadedCfg, err := config.LoadConfig(*configFile); err == nil {
				targetDir = loadedCfg.Cache.DiskDir
			}
		}
		if targetDir == "" {
			targetDir = "/var/cache/meshbeacon"
		}

		dt, err := cache.NewDiskTier(targetDir, 1*cache.GB)
		if err != nil {
			fmt.Printf("🧹 Meshbeacon: Could not open disk cache directory %s: %v\n", targetDir, err)
			os.Exit(0)
		}

		count, freed, err := dt.Clear()
		if err != nil {
			fmt.Printf("🧹 Meshbeacon: Error clearing disk cache at %s: %v\n", targetDir, err)
			os.Exit(1)
		}
		if count == 0 {
			fmt.Printf("🧹 Meshbeacon: Disk cache at %s is already empty.\n", targetDir)
		} else {
			fmt.Printf("🧹 Meshbeacon: Cleared %d cached assets (%s freed) from %s\n", count, cache.FormatSize(freed), targetDir)
		}
		os.Exit(0)
	}

	// 1. Load Configuration
	var cfg *config.Config
	if *configFile == "" {
		if *upstreamJellyfin == "" && *upstreamPlex == "" && *upstreamImmich == "" {
			if _, err := os.Stat("/etc/meshbeacon/meshbeacon.json"); err == nil {
				*configFile = "/etc/meshbeacon/meshbeacon.json"
			}
		}
	}

	if *configFile != "" {
		var err error
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig(*advertiseHost, *upstreamJellyfin, *upstreamPlex, *upstreamImmich)
	}

	// 2. Validate That At Least One Service Is Configured (unless in healthcheck mode)
	if !*healthCheck && len(cfg.Services) == 0 {
		fmt.Fprintf(os.Stderr, "🗼 Meshbeacon %s - Local LAN Media Gateway & Autodiscovery\n\n", version)
		fmt.Fprintf(os.Stderr, "Error: No upstream services configured.\n\n")
		fmt.Fprintf(os.Stderr, "Quickstart Examples:\n")
		fmt.Fprintf(os.Stderr, "  meshbeacon -jellyfin http://<server-ip>:8096\n")
		fmt.Fprintf(os.Stderr, "  meshbeacon -plex http://<server-ip>:32400\n")
		fmt.Fprintf(os.Stderr, "  meshbeacon -config /etc/meshbeacon/meshbeacon.json\n\n")
		fmt.Fprintf(os.Stderr, "Run 'meshbeacon --help' for all command-line options.\n")
		os.Exit(1)
	}

	// 3. Resolve Advertised LAN Address Dynamically if "auto" or empty
	resolvedAddr, err := netutil.NormalizeHost(cfg.AdvertiseAddr)
	if err != nil {
		log.Printf("[Warning] Could not auto-detect LAN IP: %v. Using %s", err, cfg.AdvertiseAddr)
	} else {
		cfg.AdvertiseAddr = resolvedAddr
	}

	// 4. Health Check Mode (Used by Docker HEALTHCHECK & autoheal)
	if *healthCheck {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := health.CheckAll(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "HEALTHCHECK FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("HEALTHCHECK OK: all proxies and upstreams reachable")
		os.Exit(0)
	}

	log.Printf("🗼 Meshbeacon %s - Local LAN Media Gateway & Autodiscovery", version)
	log.Printf("Advertising LAN Host: %s (%d services configured)", cfg.AdvertiseAddr, len(cfg.Services))

	// 4. Setup Context with Signal Cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Apply CLI flag overrides to cache config
	if *cacheRAM != "auto" || cfg.Cache.RAMSize == "" {
		cfg.Cache.RAMSize = *cacheRAM
	}
	if *cacheDir != "" {
		cfg.Cache.DiskDir = *cacheDir
	}
	if *cacheDisk != "auto" || cfg.Cache.DiskSize == "" {
		cfg.Cache.DiskSize = *cacheDisk
	}

	// 5. Initialize Dual-Tier Asset Cache
	var assetCache *cache.DualTierCache
	if cfg.Cache.Enabled {
		hasDisk := cfg.Cache.DiskDir != ""
		var diskBytes int64
		if hasDisk {
			var freeBytes int64
			if free, err := cache.AvailableDiskSpace(cfg.Cache.DiskDir); err == nil {
				freeBytes = free
			} else if freeParent, errParent := cache.AvailableDiskSpace(filepath.Dir(cfg.Cache.DiskDir)); errParent == nil {
				freeBytes = freeParent
			}
			autoDisk := cache.CalculateAutoDiskCacheSize(freeBytes)
			diskBytes, _ = cache.ParseSize(cfg.Cache.DiskSize, autoDisk)
		}

		totalRAM := cache.DetectTotalRAM()
		var autoRAM int64
		if hasDisk {
			autoRAM = cache.CalculateAutoRAMCacheSizeWithDisk(totalRAM)
		} else {
			autoRAM = cache.CalculateAutoRAMCacheSize(totalRAM)
		}

		ramBytes, err := cache.ParseSize(cfg.Cache.RAMSize, autoRAM)
		if err != nil {
			log.Printf("[Cache Warning] Invalid RAM size %q: %v. Using auto (%s)", cfg.Cache.RAMSize, err, cache.FormatSize(autoRAM))
			ramBytes = autoRAM
		}

		if ramBytes > 0 || (hasDisk && diskBytes > 0) {
			defaultTTL := 30 * 24 * time.Hour
			if hasDisk {
				defaultTTL = 90 * 24 * time.Hour // Bump default to 90 days when persistent disk is enabled
			}
			ttl := cache.ParseTTL(cfg.Cache.TTL, defaultTTL)
			assetCache, err = cache.NewDualTierCache(ramBytes, cfg.Cache.DiskDir, diskBytes, ttl)
			if err != nil {
				log.Printf("[Cache Warning] Failed to initialize cache: %v", err)
			} else {
				ramDesc := cache.FormatSize(ramBytes)
				if ramBytes == 0 {
					ramDesc = "disabled (pure disk)"
				}
				diskDesc := "disabled"
				if hasDisk && diskBytes > 0 {
					diskDesc = fmt.Sprintf("%s at %s", cache.FormatSize(diskBytes), cfg.Cache.DiskDir)
				}
				ttlDays := int(ttl.Hours() / 24)
				log.Printf("[Asset Cache] Active: RAM=%s (host RAM %s), Disk=%s, TTL=%dd",
					ramDesc, cache.FormatSize(totalRAM), diskDesc, ttlDays)
			}
		} else {
			log.Println("[Asset Cache] Disabled (RAM and Disk are off)")
		}
	} else {
		log.Println("[Asset Cache] Disabled")
	}

	var wg sync.WaitGroup

	// 6. Start Multi-Service HTTP Streaming Gateway
	gw, err := proxy.NewGateway(cfg.Services, assetCache)
	if err != nil {
		log.Fatalf("Failed to initialize gateway: %v", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := gw.Start(ctx); err != nil {
			log.Printf("[Gateway Error] %v", err)
		}
	}()

	// 6. Start Discovery Beacons (Both UDP probes and mDNS/Bonjour)
	for i := range cfg.Services {
		svc := &cfg.Services[i]
		var beacons []discovery.Beacon

		switch svc.Discovery.Type {
		case "jellyfin":
			// Auto-detect server ID and name from upstream if not explicitly provided
			if svc.Discovery.ServerID == "" || svc.Discovery.ServerName == "" {
				detectCtx, detectCancel := context.WithTimeout(ctx, 3*time.Second)
				id, name, err := discovery.DetectJellyfinInfo(detectCtx, svc.UpstreamURL)
				detectCancel()
				if err == nil {
					if svc.Discovery.ServerID == "" {
						svc.Discovery.ServerID = id
					}
					if svc.Discovery.ServerName == "" {
						svc.Discovery.ServerName = name
					}
				} else {
					log.Printf("[Jellyfin Beacon] Auto-detect failed for %s (%v), using fallback identity", svc.UpstreamURL, err)
					if svc.Discovery.ServerID == "" {
						svc.Discovery.ServerID = discovery.FallbackServerID(svc.UpstreamURL)
					}
					if svc.Discovery.ServerName == "" {
						svc.Discovery.ServerName = "Jellyfin"
					}
				}
			}

			targetAddr := fmt.Sprintf("%s:%d", cfg.AdvertiseAddr, svc.LocalPort)
			// UDP 7359 Probe Responder + Active Broadcaster
			beacons = append(beacons, discovery.NewJellyfinBeacon(svc.Discovery.Port, targetAddr, svc.Discovery.ServerID, svc.Discovery.ServerName))
			// mDNS / Bonjour Announcer (_jellyfin._tcp)
			beacons = append(beacons, discovery.NewMDNSBeacon(svc.Discovery.ServerName, "_jellyfin._tcp", svc.LocalPort, []string{"path=/"}))

		case "plex":
			// Auto-detect machineIdentifier from upstream if not provided
			if svc.Discovery.ServerID == "" {
				detectCtx, detectCancel := context.WithTimeout(ctx, 3*time.Second)
				id, err := discovery.DetectPlexInfo(detectCtx, svc.UpstreamURL)
				detectCancel()
				if err == nil {
					svc.Discovery.ServerID = id
				} else {
					log.Printf("[Plex Beacon] Auto-detect failed for %s (%v), using fallback identity", svc.UpstreamURL, err)
					svc.Discovery.ServerID = discovery.FallbackServerID(svc.UpstreamURL)
				}
			}
			if svc.Discovery.ServerName == "" {
				svc.Discovery.ServerName = "Plex Media Server"
			}

			// UDP 32414 GDM Probe Responder
			beacons = append(beacons, discovery.NewPlexGDMBeacon(svc.Discovery.Port, svc.Discovery.ServerName, svc.LocalPort, svc.Discovery.ServerID))
			// mDNS / Bonjour Announcer (_plexmediasvr._tcp)
			beacons = append(beacons, discovery.NewMDNSBeacon(svc.Discovery.ServerName, "_plexmediasvr._tcp", svc.LocalPort, []string{"Resource-Identifier=" + svc.Discovery.ServerID}))

		case "immich":
			serviceType := svc.Discovery.NormalizeServiceType("_immich._tcp")
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = "Immich"
			}
			if svc.Discovery.TXT == nil {
				svc.Discovery.TXT = map[string]string{"path": "/api"}
			} else if _, ok := svc.Discovery.TXT["path"]; !ok {
				svc.Discovery.TXT["path"] = "/api"
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))

		case "audiobookshelf":
			serviceType := svc.Discovery.NormalizeServiceType("_audiobookshelf._tcp")
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = "Audiobookshelf"
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))

		case "kavita":
			serviceType := svc.Discovery.NormalizeServiceType("_kavita._tcp")
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = "Kavita"
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName+" OPDS", "_opds._tcp", svc.LocalPort, []string{"path=/api/opds"}))

		case "mealie":
			serviceType := svc.Discovery.NormalizeServiceType("_mealie._tcp")
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = "Mealie"
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))

		case "navidrome", "subsonic":
			serviceType := svc.Discovery.NormalizeServiceType("_subsonic._tcp")
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = "Navidrome"
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))

		case "mdns", "generic":
			serviceType := svc.Discovery.NormalizeServiceType(svc.Name)
			displayName := svc.Discovery.ServerName
			if displayName == "" {
				displayName = svc.Name
			}
			txt := svc.Discovery.FormattedTXT()
			beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, txt))

		default:
			continue
		}

		for _, b := range beacons {
			wg.Add(1)
			go func(beacon discovery.Beacon) {
				defer wg.Done()
				if err := beacon.Start(ctx); err != nil {
					log.Printf("[%s Error] %v", beacon.Name(), err)
				}
			}(b)
		}
	}

	log.Println("🚀 Meshbeacon is live, proxying streams and broadcasting on LAN.")
	<-ctx.Done()
	log.Println("\n[Shutdown] Signal received, stopping all services...")

	wg.Wait()
	log.Println("👋 Meshbeacon shutdown complete.")
}
