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

	"github.com/shimco/tailbeacon/internal/config"
	"github.com/shimco/tailbeacon/internal/discovery"
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
		advertiseHost    = flag.String("advertise-host", getEnv("ADVERTISE_HOST", "http://192.168.100.2"), "Local host/IP advertised to LAN clients")
		upstreamJellyfin = flag.String("jellyfin", getEnv("UPSTREAM_JELLYFIN", "http://mediaserver.horse-pancake.ts.net:8096"), "Upstream Jellyfin URL")
		upstreamPlex     = flag.String("plex", getEnv("UPSTREAM_PLEX", "http://mediaserver.horse-pancake.ts.net:32400"), "Upstream Plex URL")
	)
	flag.Parse()

	log.Println("🗼 Tailbeacon - Local LAN Media Gateway & Autodiscovery")

	// 1. Load or Generate Configuration
	var cfg *config.Config
	if *configFile != "" {
		var err error
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		log.Printf("Loaded configuration from %s (%d services)", *configFile, len(cfg.Services))
	} else {
		cfg = config.DefaultConfig(*advertiseHost, *upstreamJellyfin, *upstreamPlex)
		log.Printf("Using default configuration for %d services", len(cfg.Services))
	}

	// 2. Setup Context with Signal Cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// 3. Start Multi-Service HTTP Streaming Gateway
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

	// 4. Start Discovery Beacons
	for _, svc := range cfg.Services {
		var b discovery.Beacon

		switch svc.Discovery.Type {
		case "jellyfin":
			targetAddr := fmt.Sprintf("%s:%d", cfg.AdvertiseAddr, svc.LocalPort)
			b = discovery.NewJellyfinBeacon(svc.Discovery.Port, targetAddr, svc.Discovery.ServerID, svc.Discovery.ServerName)

		case "plex":
			b = discovery.NewPlexGDMBeacon(svc.Discovery.Port, svc.Discovery.ServerName, svc.LocalPort, svc.Discovery.ServerID)

		default:
			continue
		}

		if b != nil {
			wg.Add(1)
			go func(beacon discovery.Beacon) {
				defer wg.Done()
				if err := beacon.Start(ctx); err != nil {
					log.Printf("[%s Error] %v", beacon.Name(), err)
				}
			}(b)
		}
	}

	log.Println("🚀 Tailbeacon is live and broadcasting on local network.")
	<-ctx.Done()
	log.Println("\n[Shutdown] Signal received, stopping all services...")

	wg.Wait()
	log.Println("👋 Tailbeacon shutdown complete.")
}
