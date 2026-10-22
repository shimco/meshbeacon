package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/shimco/meshbeacon/internal/config"
)

// CheckService performs TCP and upstream reachability checks for a configured service.
func CheckService(ctx context.Context, svc config.ServiceConfig) error {
	// 1. Check local TCP listener
	localAddr := fmt.Sprintf("127.0.0.1:%d", svc.LocalPort)
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", localAddr)
	if err != nil {
		return fmt.Errorf("local port %d is not responding: %w", svc.LocalPort, err)
	}
	conn.Close()

	// 2. Check upstream HTTP reachability
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, svc.UpstreamURL, nil)
	if err != nil {
		return fmt.Errorf("invalid upstream URL %s: %w", svc.UpstreamURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream %s unreachable: %w", svc.UpstreamURL, err)
	}
	defer resp.Body.Close()

	return nil
}

// CheckAll executes health checks against all configured services.
func CheckAll(ctx context.Context, cfg *config.Config) error {
	for _, svc := range cfg.Services {
		if err := CheckService(ctx, svc); err != nil {
			return fmt.Errorf("[%s] health check failed: %w", svc.Name, err)
		}
	}
	return nil
}
