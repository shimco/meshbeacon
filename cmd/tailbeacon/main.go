package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shimco/tailbeacon/internal/config"
	"github.com/shimco/tailbeacon/internal/discovery"
	"github.com/shimco/tailbeacon/internal/health"
	"github.com/shimco/tailbeacon/internal/netutil"
	"github.com/shimco/tailbeacon/internal/proxy"
)

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
		upstreamJellyfin = flag.String("jellyfin", getEnv("UPSTREAM_JELLYFIN", "http://mediaserver.horse-pancake.ts.net:8096"), "Upstream Jellyfin URL")
		upstreamPlex     = flag.String("plex", getEnv("UPSTREAM_PLEX", "http://mediaserver.horse-pancake.ts.net:32400"), "Upstream Plex URL")
		healthCheck      = flag.Bool("health-check", false, "Run health check against configured services and exit")
	)
	flag.Parse()

	// 1. Load Configuration
	var cfg *config.Config
	if *configFile != "" {
		var err error
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig(*advertiseHost, *upstreamJellyfin, *upstreamPlex)
	}

	// 2. Resolve Advertised LAN Address Dynamically if "auto" or empty
	resolvedAddr, err := netutil.NormalizeHost(cfg.AdvertiseAddr)
	if err != nil {
		log.Printf("[Warning] Could not auto-detect LAN IP: %v. Using %s", err, cfg.AdvertiseAddr)
	} else {
		cfg.AdvertiseAddr = resolvedAddr
	}

	// 3. Health Check Mode (Used by Docker HEALTHCHECK & autoheal)
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

	log.Println("🗼 Tailbeacon - Local LAN Media Gateway & Autodiscovery")
	log.Printf("Advertising LAN Host: %s (%d services configured)", cfg.AdvertiseAddr, len(cfg.Services))

	// 4. Setup Context with Signal Cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// 5. Start Multi-Service HTTP Streaming Gateway
	gw, err := proxy.NewGateway(cfg.Services)
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
	for _, svc := range cfg.Services {
		var beacons []discovery.Beacon

		switch svc.Discovery.Type {
		case "jellyfin":
			targetAddr := fmt.Sprintf("%s:%d", cfg.AdvertiseAddr, svc.LocalPort)
			// UDP 7359 Probe Responder
			beacons = append(beacons, discovery.NewJellyfinBeacon(svc.Discovery.Port, targetAddr, svc.Discovery.ServerID, svc.Discovery.ServerName))
			// mDNS / Bonjour Announcer (_jellyfin._tcp)
			beacons = append(beacons, discovery.NewMDNSBeacon(svc.Discovery.ServerName, "_jellyfin._tcp", svc.LocalPort, []string{"path=/"}))

		case "plex":
			// UDP 32414 GDM Probe Responder
			beacons = append(beacons, discovery.NewPlexGDMBeacon(svc.Discovery.Port, svc.Discovery.ServerName, svc.LocalPort, svc.Discovery.ServerID))
			// mDNS / Bonjour Announcer (_plexmediasvr._tcp)
			beacons = append(beacons, discovery.NewMDNSBeacon(svc.Discovery.ServerName, "_plexmediasvr._tcp", svc.LocalPort, []string{"Resource-Identifier=" + svc.Discovery.ServerID}))

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

	log.Println("🚀 Tailbeacon is live, proxying streams and broadcasting on LAN.")
	<-ctx.Done()
	log.Println("\n[Shutdown] Signal received, stopping all services...")

	wg.Wait()
	log.Println("👋 Tailbeacon shutdown complete.")
}
