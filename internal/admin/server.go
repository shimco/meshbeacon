package admin

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shimco/meshbeacon/internal/cache"
	"github.com/shimco/meshbeacon/internal/config"
	"github.com/shimco/meshbeacon/internal/proxy"
	"github.com/shimco/meshbeacon/internal/updater"
)

//go:embed web/index.html
var indexHTML []byte

// Server serves the embedded management dashboard, setup wizard, and REST API.
type Server struct {
	mu         sync.RWMutex
	cfg        *config.Config
	configPath string
	cache      *cache.DualTierCache
	telemetry  *proxy.Telemetry
	version    string
	startTime  time.Time
	reloadFn   func(*config.Config) error
	httpServer *http.Server
}

// NewServer initializes the admin and setup HTTP server.
func NewServer(cfg *config.Config, configPath, version string, c *cache.DualTierCache, tel *proxy.Telemetry, reloadFn func(*config.Config) error) *Server {
	s := &Server{
		cfg:        cfg,
		configPath: configPath,
		cache:      c,
		telemetry:  tel,
		version:    version,
		startTime:  time.Now(),
		reloadFn:   reloadFn,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/cache/clear", s.handleCacheClear)
	mux.HandleFunc("/api/update", s.handleUpdate)

	port := cfg.AdminPort
	if port == 0 {
		port = 7350
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

// UpdateConfig updates the in-memory config reference.
func (s *Server) UpdateConfig(newCfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = newCfg
}

// Start listens and serves until ctx is canceled.
func (s *Server) Start(ctx context.Context) error {
	errChan := make(chan error, 1)
	go func() {
		log.Printf("[Web Dashboard] Management & setup portal active on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type StatusResponse struct {
	Version       string                 `json:"version"`
	Mode          string                 `json:"mode"`
	Configured    bool                   `json:"configured"`
	AdvertiseAddr string                 `json:"advertise_addr"`
	Uptime        string                 `json:"uptime"`
	Services      []config.ServiceConfig `json:"services"`
	Cache         map[string]interface{} `json:"cache"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	configured := cfg.IsConfigured()
	mode := "active"
	if !configured {
		mode = "standby"
	}

	uptimeDur := time.Since(s.startTime).Truncate(time.Second)
	uptimeStr := uptimeDur.String()

	cacheInfo := map[string]interface{}{
		"enabled":  cfg.Cache.Enabled,
		"ram_size": cfg.Cache.RAMSize,
		"hits":     0,
		"misses":   0,
	}

	if s.cache != nil {
		stats := s.cache.Stats()
		cacheInfo["hits"] = stats.Hits
		cacheInfo["misses"] = stats.Misses
		cacheInfo["ram_used"] = cache.FormatSize(stats.RAMBytes)
	}

	resp := StatusResponse{
		Version:       s.version,
		Mode:          mode,
		Configured:    configured,
		AdvertiseAddr: cfg.AdvertiseAddr,
		Uptime:        uptimeStr,
		Services:      cfg.Services,
		Cache:         cacheInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		cfg := s.cfg
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)

	case http.MethodPost:
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid json: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if err := newCfg.Validate(); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"validation failed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		destPath := s.configPath
		if destPath == "" {
			destPath = "/etc/meshbeacon/meshbeacon.json"
		}

		if err := config.SaveConfig(destPath, &newCfg); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to save config: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		s.UpdateConfig(&newCfg)

		if s.reloadFn != nil {
			if err := s.reloadFn(&newCfg); err != nil {
				log.Printf("[Admin] Config reloaded with warnings: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		rel, isNewer, err := updater.CheckLatestRelease(ctx, s.version)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current":  s.version,
			"latest":   rel.TagName,
			"is_newer": isNewer,
			"url":      rel.HTMLURL,
		})

	case http.MethodPost:
		isManaged, reason := updater.IsManagedEnvironment()
		if isManaged {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, reason), http.StatusForbidden)
			return
		}

		// Check latest release
		rel, isNewer, err := updater.CheckLatestRelease(ctx, s.version)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if !isNewer {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Already up to date",
			})
			return
		}

		// Perform safe update
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer updateCancel()

		if err := updater.ExecuteUpdate(updateCtx, s.version, s.configPath); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"update failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Successfully updated to %s! Restarting service...", rel.TagName),
		})

		// Trigger service reload/restart if running via OpenWrt init script or exit gracefully
		go func() {
			time.Sleep(1 * time.Second)
			log.Println("[Updater] Update applied successfully. Exiting for service supervisor reload...")
			os.Exit(0)
		}()

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.cache != nil {
		s.cache.Clear()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Asset cache cleared",
	})
}
