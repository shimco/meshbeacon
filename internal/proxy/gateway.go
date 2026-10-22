package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/shimco/tailbeacon/internal/config"
)

// Gateway manages a collection of concurrent reverse proxies across multiple ports.
type Gateway struct {
	services []config.ServiceConfig
	servers  []*http.Server
	proxies  []*StreamProxy
}

// NewGateway creates a gateway from service configs.
func NewGateway(services []config.ServiceConfig) (*Gateway, error) {
	gw := &Gateway{
		services: services,
		servers:  make([]*http.Server, 0, len(services)),
		proxies:  make([]*StreamProxy, 0, len(services)),
	}

	for _, svc := range services {
		rp, err := NewStreamProxy(svc.Name, svc.UpstreamURL)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize proxy for %s (%s): %w", svc.Name, svc.UpstreamURL, err)
		}

		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", svc.LocalPort),
			Handler: rp,
		}
		gw.servers = append(gw.servers, server)
		gw.proxies = append(gw.proxies, rp)
	}

	return gw, nil
}

// Start runs all service proxy listeners concurrently until ctx is canceled.
func (gw *Gateway) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	for i, server := range gw.servers {
		svc := gw.services[i]
		wg.Add(1)

		go func(s *http.Server, name string, port int) {
			defer wg.Done()
			log.Printf("[Gateway] %s proxy listening on :%d", name, port)
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[Gateway Error] %s server failed: %v", name, err)
			}
		}(server, svc.Name, svc.LocalPort)
	}

	// Wait for context cancellation to initiate shutdown
	<-ctx.Done()
	log.Println("[Gateway] Shutting down all proxy servers...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, server := range gw.servers {
		_ = server.Shutdown(shutdownCtx)
	}

	wg.Wait()
	log.Println("[Gateway] All proxy servers stopped cleanly.")
	return nil
}
