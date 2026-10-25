package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/shimco/meshbeacon/internal/admin"
	"github.com/shimco/meshbeacon/internal/cache"
	"github.com/shimco/meshbeacon/internal/config"
	"github.com/shimco/meshbeacon/internal/discovery"
	"github.com/shimco/meshbeacon/internal/health"
	"github.com/shimco/meshbeacon/internal/logring"
	"github.com/shimco/meshbeacon/internal/netutil"
	"github.com/shimco/meshbeacon/internal/proxy"
	"github.com/shimco/meshbeacon/internal/updater"
)

var version = "v0.8.4"

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func printStandbyBanner(host string, port int, configPath string) {
	fmt.Fprintf(os.Stderr, "\n┌────────────────────────────────────────────────────────────────────────┐\n")
	fmt.Fprintf(os.Stderr, "│  🗼 Meshbeacon %-7s is running in STANDBY MODE (Setup Required)     │\n", version)
	fmt.Fprintf(os.Stderr, "├────────────────────────────────────────────────────────────────────────┤\n")
	fmt.Fprintf(os.Stderr, "│                                                                        │\n")
	fmt.Fprintf(os.Stderr, "│  No upstream services configured yet.                                  │\n")
	fmt.Fprintf(os.Stderr, "│  Configure via Web UI:   http://%-38s │\n", fmt.Sprintf("%s:%d", host, port))
	fmt.Fprintf(os.Stderr, "│  Or via local network:   http://meshbeacon.local:%-23d │\n", port)
	if configPath != "" {
		fmt.Fprintf(os.Stderr, "│  Or edit file:           %-45s │\n", configPath)
	}
	fmt.Fprintf(os.Stderr, "│                                                                        │\n")
	fmt.Fprintf(os.Stderr, "└────────────────────────────────────────────────────────────────────────┘\n\n")
}

func main() {
	var (
		configFile       = flag.String("config", getEnv("CONFIG_FILE", ""), "Path to JSON configuration file")
		advertiseHost    = flag.String("advertise-host", getEnv("ADVERTISE_HOST", "auto"), "Local host/IP advertised to LAN clients ('auto' to detect)")
		upstreamJellyfin = flag.String("jellyfin", getEnv("UPSTREAM_JELLYFIN", ""), "Upstream Jellyfin URL (e.g. http://media-server:8096)")
		upstreamPlex     = flag.String("plex", getEnv("UPSTREAM_PLEX", ""), "Upstream Plex URL (e.g. http://media-server:32400)")
		upstreamImmich   = flag.String("immich", getEnv("UPSTREAM_IMMICH", ""), "Upstream Immich URL (e.g. http://media-server:2283)")
		cacheRAM         = flag.String("cache-ram", getEnv("CACHE_RAM", "auto"), "Asset cache RAM size ('auto', '64MB', 'off')")
		cacheDir         = flag.String("cache-dir", getEnv("CACHE_DIR", ""), "Optional persistent disk cache directory (default off)")
		cacheDisk        = flag.String("cache-disk-size", getEnv("CACHE_DISK_SIZE", "auto"), "Max disk cache size if cache-dir is enabled ('auto', '2GB')")
		accessLog        = flag.Bool("access-log", false, "Log each incoming gateway request to stdout in real-time (default false to prevent SD-card wear)")
		healthCheck      = flag.Bool("health-check", false, "Run health check against configured services and exit")
		clearCache       = flag.Bool("clear-cache", false, "Clear all cached assets from disk and exit")
		testConfig       = flag.Bool("test-config", false, "Validate configuration file syntax and schema, then exit")
		checkUpdate      = flag.Bool("check-update", false, "Check for available updates on GitHub and exit")
		doUpdate         = flag.Bool("update", false, "Self-update meshbeacon to the latest release and exit")
		adminPort        = flag.Int("admin-port", 0, "Web dashboard and setup portal port (default 7350)")
		showVersion      = flag.Bool("version", false, "Print Meshbeacon version and exit")
	)
	flag.Parse()

	logRing := logring.New(250)
	log.SetOutput(io.MultiWriter(os.Stderr, logRing))

	if *showVersion {
		fmt.Printf("meshbeacon %s\n", version)
		os.Exit(0)
	}

	// 1. Check Update Mode
	if *checkUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rel, isNewer, err := updater.CheckLatestRelease(ctx, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking update: %v\n", err)
			os.Exit(1)
		}
		if isNewer {
			fmt.Printf("Update available: %s (current: %s)\nRelease URL: %s\nRun 'meshbeacon -update' to install.\n", rel.TagName, version, rel.HTMLURL)
		} else {
			fmt.Printf("Meshbeacon is up to date (%s).\n", version)
		}
		os.Exit(0)
	}

	// 2. Perform Self-Update Mode
	if *doUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		fmt.Printf("Checking and updating Meshbeacon (current: %s)...\n", version)
		if err := updater.ExecuteUpdate(ctx, version, *configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Update completed successfully! Please restart meshbeacon.")
		os.Exit(0)
	}

	// 3. Cache Clearing Mode
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

	// 4. Locate & Load Configuration
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
			if *testConfig {
				fmt.Fprintf(os.Stderr, "CONFIGURATION LOAD ERROR: %v\n", err)
				os.Exit(1)
			}
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig(*advertiseHost, *upstreamJellyfin, *upstreamPlex, *upstreamImmich)
	}

	if *adminPort > 0 {
		cfg.AdminPort = *adminPort
	}
	if cfg.AdminPort == 0 {
		cfg.AdminPort = 7350
	}

	// 5. Test Configuration Mode
	if *testConfig {
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "CONFIGURATION VALIDATION FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("CONFIGURATION VALID: schema_version=%d, admin_port=%d, services=%d\n", cfg.SchemaVersion, cfg.AdminPort, len(cfg.Services))
		os.Exit(0)
	}

	// 6. Resolve Advertised LAN Address Dynamically if "auto" or empty
	resolvedAddr, err := netutil.NormalizeHost(cfg.AdvertiseAddr)
	if err != nil {
		log.Printf("[Warning] Could not auto-detect LAN IP: %v. Using %s", err, cfg.AdvertiseAddr)
	} else {
		cfg.AdvertiseAddr = resolvedAddr
	}

	// 7. Health Check Mode (Used by Docker HEALTHCHECK & autoheal)
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

	// 8. Setup Context with Signal Cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var (
		wg          sync.WaitGroup
		gwCtx       context.Context
		gwCancel    context.CancelFunc
		assetCache  *cache.DualTierCache
		adminServer *admin.Server
		tel         = proxy.NewTelemetry(*accessLog)
	)

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

	// Function to start or re-start gateway and discovery services
	startGatewayAndBeacons := func(currentCfg *config.Config) {
		if gwCancel != nil {
			gwCancel()
		}
		gwCtx, gwCancel = context.WithCancel(ctx)

		// Initialize or Re-initialize Cache
		if currentCfg.Cache.Enabled {
			hasDisk := currentCfg.Cache.DiskDir != ""
			var diskBytes int64
			if hasDisk {
				var freeBytes int64
				if free, err := cache.AvailableDiskSpace(currentCfg.Cache.DiskDir); err == nil {
					freeBytes = free
				} else if freeParent, errParent := cache.AvailableDiskSpace(filepath.Dir(currentCfg.Cache.DiskDir)); errParent == nil {
					freeBytes = freeParent
				}
				autoDisk := cache.CalculateAutoDiskCacheSize(freeBytes)
				diskBytes, _ = cache.ParseSize(currentCfg.Cache.DiskSize, autoDisk)
			}

			totalRAM := cache.DetectTotalRAM()
			var autoRAM int64
			if hasDisk {
				autoRAM = cache.CalculateAutoRAMCacheSizeWithDisk(totalRAM)
			} else {
				autoRAM = cache.CalculateAutoRAMCacheSize(totalRAM)
			}

			ramBytes, err := cache.ParseSize(currentCfg.Cache.RAMSize, autoRAM)
			if err != nil {
				log.Printf("[Cache Warning] Invalid RAM size %q: %v. Using auto (%s)", currentCfg.Cache.RAMSize, err, cache.FormatSize(autoRAM))
				ramBytes = autoRAM
			}

			if ramBytes > 0 || (hasDisk && diskBytes > 0) {
				defaultTTL := 30 * 24 * time.Hour
				if hasDisk {
					defaultTTL = 90 * 24 * time.Hour
				}
				ttl := cache.ParseTTL(currentCfg.Cache.TTL, defaultTTL)
				newCache, err := cache.NewDualTierCache(ramBytes, currentCfg.Cache.DiskDir, diskBytes, ttl)
				if err != nil {
					log.Printf("[Cache Warning] Failed to initialize cache: %v", err)
				} else {
					assetCache = newCache
					if adminServer != nil {
						adminServer.UpdateCache(assetCache)
					}
					ramDesc := cache.FormatSize(ramBytes)
					if ramBytes == 0 {
						ramDesc = "disabled (pure disk)"
					}
					diskDesc := "disabled"
					if hasDisk && diskBytes > 0 {
						diskDesc = fmt.Sprintf("%s at %s", cache.FormatSize(diskBytes), currentCfg.Cache.DiskDir)
					}
					ttlDays := int(ttl.Hours() / 24)
					log.Printf("[Asset Cache] Active: RAM=%s (host RAM %s), Disk=%s, TTL=%dd",
						ramDesc, cache.FormatSize(totalRAM), diskDesc, ttlDays)
				}
			}
		} else {
			assetCache = nil
			if adminServer != nil {
				adminServer.UpdateCache(nil)
			}
			log.Println("[Asset Cache] Disabled")
		}

		// Initialize Gateway
		gw, err := proxy.NewGateway(currentCfg.Services, assetCache, tel)
		if err != nil {
			log.Printf("[Gateway Error] Could not initialize gateway: %v", err)
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := gw.Start(gwCtx); err != nil {
				log.Printf("[Gateway Error] %v", err)
			}
		}()

		// Start Discovery Beacons
		for i := range currentCfg.Services {
			svc := &currentCfg.Services[i]
			var beacons []discovery.Beacon

			switch svc.Discovery.Type {
			case "jellyfin":
				if svc.Discovery.ServerID == "" || svc.Discovery.ServerName == "" {
					detectCtx, detectCancel := context.WithTimeout(gwCtx, 3*time.Second)
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
						if svc.Discovery.ServerID == "" {
							svc.Discovery.ServerID = discovery.FallbackServerID(svc.UpstreamURL)
						}
						if svc.Discovery.ServerName == "" {
							svc.Discovery.ServerName = "Jellyfin"
						}
					}
				}
				targetAddr := fmt.Sprintf("%s:%d", currentCfg.AdvertiseAddr, svc.LocalPort)
				beacons = append(beacons, discovery.NewJellyfinBeacon(svc.Discovery.Port, targetAddr, svc.Discovery.ServerID, svc.Discovery.ServerName))
				beacons = append(beacons, discovery.NewMDNSBeacon(svc.Discovery.ServerName, "_jellyfin._tcp", svc.LocalPort, []string{"path=/"}))

			case "plex":
				if svc.Discovery.ServerID == "" {
					detectCtx, detectCancel := context.WithTimeout(gwCtx, 3*time.Second)
					id, err := discovery.DetectPlexInfo(detectCtx, svc.UpstreamURL)
					detectCancel()
					if err == nil {
						svc.Discovery.ServerID = id
					} else {
						svc.Discovery.ServerID = discovery.FallbackServerID(svc.UpstreamURL)
					}
				}
				if svc.Discovery.ServerName == "" {
					svc.Discovery.ServerName = "Plex Media Server"
				}
				beacons = append(beacons, discovery.NewPlexGDMBeacon(svc.Discovery.Port, svc.Discovery.ServerName, svc.LocalPort, svc.Discovery.ServerID))
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
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, serviceType, svc.LocalPort, svc.Discovery.FormattedTXT()))

			case "audiobookshelf":
				displayName := svc.Discovery.ServerName
				if displayName == "" {
					displayName = "Audiobookshelf"
				}
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, "_audiobookshelf._tcp", svc.LocalPort, svc.Discovery.FormattedTXT()))

			case "kavita":
				displayName := svc.Discovery.ServerName
				if displayName == "" {
					displayName = "Kavita"
				}
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, "_kavita._tcp", svc.LocalPort, svc.Discovery.FormattedTXT()))
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName+" OPDS", "_opds._tcp", svc.LocalPort, []string{"path=/api/opds"}))

			case "mealie":
				displayName := svc.Discovery.ServerName
				if displayName == "" {
					displayName = "Mealie"
				}
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, "_mealie._tcp", svc.LocalPort, svc.Discovery.FormattedTXT()))

			case "navidrome", "subsonic":
				displayName := svc.Discovery.ServerName
				if displayName == "" {
					displayName = "Navidrome"
				}
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, "_subsonic._tcp", svc.LocalPort, svc.Discovery.FormattedTXT()))

			case "mdns", "generic":
				displayName := svc.Discovery.ServerName
				if displayName == "" {
					displayName = svc.Name
				}
				beacons = append(beacons, discovery.NewMDNSBeacon(displayName, svc.Discovery.NormalizeServiceType(svc.Name), svc.LocalPort, svc.Discovery.FormattedTXT()))
			}

			for _, b := range beacons {
				wg.Add(1)
				go func(beacon discovery.Beacon) {
					defer wg.Done()
					if err := beacon.Start(gwCtx); err != nil {
						log.Printf("[%s Error] %v", beacon.Name(), err)
					}
				}(b)
			}
		}
		log.Printf("🚀 Meshbeacon is actively proxying %d services on LAN (%s)", len(currentCfg.Services), currentCfg.AdvertiseAddr)
	}

	// 9. Initialize Web Dashboard & Setup Portal
	adminServer = admin.NewServer(cfg, *configFile, version, assetCache, tel, logRing, func(newCfg *config.Config) error {
		log.Println("[Admin] Configuration update received via Web UI.")
		cfg = newCfg
		if cfg.IsConfigured() {
			startGatewayAndBeacons(cfg)
		}
		return nil
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := adminServer.Start(ctx); err != nil {
			log.Printf("[Admin Error] %v", err)
		}
	}()

	// 10. Start Admin mDNS Beacon (_meshbeacon._tcp) so meshbeacon.local:7350 resolves
	adminMDNS := discovery.NewMDNSBeacon("Meshbeacon", "_meshbeacon._tcp", cfg.AdminPort, []string{"path=/"})
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = adminMDNS.Start(ctx)
	}()

	// 11. Evaluate Initial Mode: Standby vs Active
	if !cfg.IsConfigured() {
		printStandbyBanner(cfg.AdvertiseAddr, cfg.AdminPort, *configFile)
	} else {
		log.Printf("🗼 Meshbeacon %s - Local LAN Media Gateway & Autodiscovery", version)
		log.Printf("Advertising LAN Host: %s (%d services configured)", cfg.AdvertiseAddr, len(cfg.Services))
		startGatewayAndBeacons(cfg)
	}

	// 12. Periodic Background Auto-Update Checker (24-hour cycle with random jitter)
	if cfg.AutoUpdate {
		go func() {
			jitter := time.Duration(rand.Intn(1800)) * time.Second
			select {
			case <-ctx.Done():
				return
			case <-time.After(5*time.Minute + jitter):
			}

			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()

			check := func() {
				checkCtx, checkCancel := context.WithTimeout(ctx, 30*time.Second)
				defer checkCancel()
				rel, isNewer, err := updater.CheckLatestRelease(checkCtx, version)
				if err == nil && isNewer {
					log.Printf("[Auto-Update] Newer release %s discovered (current %s). Applying safe update...", rel.TagName, version)
					updateCtx, updateCancel := context.WithTimeout(ctx, 2*time.Minute)
					defer updateCancel()
					if err := updater.ExecuteUpdate(updateCtx, version, *configFile); err != nil {
						log.Printf("[Auto-Update Warning] Could not apply update: %v", err)
					} else {
						log.Printf("[Auto-Update] Successfully updated to %s. Triggering service reload...", rel.TagName)
						os.Exit(0)
					}
				}
			}

			check()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					check()
				}
			}
		}()
	}

	<-ctx.Done()
	log.Println("\n[Shutdown] Signal received, stopping all services...")
	if gwCancel != nil {
		gwCancel()
	}
	wg.Wait()
	log.Println("👋 Meshbeacon shutdown complete.")
}
