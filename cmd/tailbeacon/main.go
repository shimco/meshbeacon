package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		upstreamURL   = flag.String("upstream", getEnv("UPSTREAM_URL", "http://mediaserver.horse-pancake.ts.net:8096"), "Upstream Tailscale media server URL")
		localPort     = flag.Int("port", 8096, "Local port to listen on for proxy traffic")
		discoveryPort = flag.Int("discovery-port", 7359, "UDP port for Jellyfin discovery broadcast")
		advertiseAddr = flag.String("advertise-addr", getEnv("ADVERTISE_ADDR", ""), "Local address advertised to clients (e.g., http://192.168.1.50:8096)")
		serverName    = flag.String("name", getEnv("SERVER_NAME", "Mediaserver"), "Friendly name broadcast to local clients")
		serverId      = flag.String("id", getEnv("SERVER_ID", "4753126c145c43db84cc5fd0fd164aeb"), "Unique Server ID for Jellyfin clients")
		enableBeacon  = flag.Bool("beacon", true, "Enable UDP LAN auto-discovery responder")
	)
	flag.Parse()

	log.Println("🗼 Starting Tailbeacon - Local LAN Media Gateway")
	log.Printf("Upstream Peer:   %s", *upstreamURL)
	log.Printf("Local HTTP Port: %d", *localPort)

	// Determine advertisement address if not explicitly passed
	finalAdvertise := *advertiseAddr
	if finalAdvertise == "" {
		finalAdvertise = fmt.Sprintf("http://localhost:%d", *localPort)
	}

	// 1. Initialize Reverse Proxy
	rp, err := proxy.NewStreamProxy(*upstreamURL)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *localPort),
		Handler: rp,
	}

	// 2. Start HTTP Proxy in goroutine
	go func() {
		log.Printf("[Gateway] HTTP proxy listening on :%d", *localPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 3. Initialize & Start Discovery Beacon
	var beacon *discovery.JellyfinBeacon
	if *enableBeacon {
		beacon = discovery.NewJellyfinBeacon(*discoveryPort, finalAdvertise, *serverId, *serverName)
		if err := beacon.Start(); err != nil {
			log.Printf("[Warning] Discovery beacon failed to start: %v", err)
		}
	}

	// 4. Graceful Shutdown on Interrupt
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("\n[Shutdown] Shutting down Tailbeacon gracefully...")

	if beacon != nil {
		beacon.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[Shutdown] Server shutdown error: %v", err)
	}

	log.Println("[Shutdown] Tailbeacon stopped.")
}
