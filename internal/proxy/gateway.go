package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shimco/meshbeacon/internal/cache"
	"github.com/shimco/meshbeacon/internal/config"
)

type httpEntry struct {
	server *http.Server
	name   string
	port   int
}

// Gateway manages a collection of concurrent reverse proxies across multiple ports.
type Gateway struct {
	services   []config.ServiceConfig
	httpSvcs   []httpEntry
	proxies    []*StreamProxy
	tcpProxies []*TCPProxy
	cache      *cache.DualTierCache
	telemetry  *Telemetry
}

// isTCPService checks if a service requires raw TCP stream proxying instead of HTTP reverse proxying.
func isTCPService(svc config.ServiceConfig) bool {
	u := strings.ToLower(svc.UpstreamURL)
	if strings.HasPrefix(u, "tcp://") || strings.HasPrefix(u, "smb://") {
		return true
	}
	t := strings.ToLower(svc.Discovery.Type)
	if t == "samba" || t == "smb" || svc.LocalPort == 445 {
		return true
	}
	return false
}

// parseTCPTarget extracts host:port from upstream URL for raw TCP dial.
func parseTCPTarget(raw string, defaultPort int) string {
	raw = strings.TrimPrefix(raw, "tcp://")
	raw = strings.TrimPrefix(raw, "smb://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimRight(raw, "/")
	if !strings.Contains(raw, ":") {
		return fmt.Sprintf("%s:%d", raw, defaultPort)
	}
	return raw
}

// NewGateway creates a gateway from service configs.
func NewGateway(services []config.ServiceConfig, assetCache *cache.DualTierCache, tel *Telemetry) (*Gateway, error) {
	gw := &Gateway{
		services:   services,
		httpSvcs:   make([]httpEntry, 0, len(services)),
		proxies:    make([]*StreamProxy, 0, len(services)),
		tcpProxies: make([]*TCPProxy, 0),
		cache:      assetCache,
		telemetry:  tel,
	}

	for _, svc := range services {
		if svc.Discovery.Direct && svc.LocalPort == 0 {
			// Discovery-only mode: do not spin up proxy listener
			continue
		}
		if isTCPService(svc) {
			target := parseTCPTarget(svc.UpstreamURL, 445)
			tp := NewTCPProxy(svc.Name, svc.LocalPort, target)
			gw.tcpProxies = append(gw.tcpProxies, tp)
			continue
		}

		rp, err := NewStreamProxy(svc.Name, svc.UpstreamURL, assetCache, tel)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize proxy for %s (%s): %w", svc.Name, svc.UpstreamURL, err)
		}

		server := &http.Server{
			Addr:              fmt.Sprintf(":%d", svc.LocalPort),
			Handler:           rp,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		gw.httpSvcs = append(gw.httpSvcs, httpEntry{
			server: server,
			name:   svc.Name,
			port:   svc.LocalPort,
		})
		gw.proxies = append(gw.proxies, rp)
	}

	return gw, nil
}

// Start runs all service proxy listeners concurrently until ctx is canceled.
func (gw *Gateway) Start(ctx context.Context) error {
	if gw.telemetry != nil {
		gw.telemetry.StartHeartbeat(ctx, 5*time.Minute)
	}

	var wg sync.WaitGroup

	// Start raw TCP stream proxies (e.g. Samba 445)
	for _, tp := range gw.tcpProxies {
		wg.Add(1)
		go func(p *TCPProxy) {
			defer wg.Done()
			if err := p.Start(ctx); err != nil {
				log.Printf("[TCP Gateway Error] %v", err)
			}
		}(tp)
	}

	// Start HTTP streaming reverse proxies
	for _, entry := range gw.httpSvcs {
		wg.Add(1)
		go func(e httpEntry) {
			defer wg.Done()
			log.Printf("[Gateway] %s proxy listening on :%d", e.name, e.port)
			if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[Gateway Error] %s server failed: %v", e.name, err)
			}
		}(entry)
	}

	// Wait for context cancellation to initiate shutdown
	<-ctx.Done()
	log.Println("[Gateway] Shutting down all proxy servers...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, entry := range gw.httpSvcs {
		_ = entry.server.Shutdown(shutdownCtx)
	}

	wg.Wait()
	log.Println("[Gateway] All proxy servers stopped cleanly.")
	return nil
}
