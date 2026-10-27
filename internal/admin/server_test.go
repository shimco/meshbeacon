package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shimco/meshbeacon/internal/config"
	"github.com/shimco/meshbeacon/internal/logring"
)

func TestAdminEndpoints(t *testing.T) {
	cfg := &config.Config{
		SchemaVersion: 1,
		AdminPort:     7350,
		AdvertiseAddr: "192.168.88.1",
		Services:      []config.ServiceConfig{},
	}

	ring := logring.New(10)
	_, _ = ring.Write([]byte("test log entry\n"))
	srv := NewServer(cfg, "", "v0.8.5", nil, nil, ring, nil)

	// 1. Test Index
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	srv.handleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /, got %d", rec.Code)
	}

	// 2. Test Healthz
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	srv.handleHealthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /healthz, got %d", rec.Code)
	}

	// 3. Test Status in Standby Mode
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/status", nil)
	srv.handleStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /api/status, got %d", rec.Code)
	}

	var status StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode status response: %v", err)
	}
	if status.Mode != "standby" {
		t.Errorf("Expected mode 'standby', got %s", status.Mode)
	}
	if status.Configured {
		t.Errorf("Expected configured=false for empty services")
	}

	// 4. Test Config Save Validation
	badConfig := []byte(`{"services":[{"name":"","local_port":0}]}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/config", bytes.NewReader(badConfig))
	srv.handleConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid config, got %d", rec.Code)
	}
}

func TestBasePathAndReverseProxy(t *testing.T) {
	cfg := &config.Config{
		SchemaVersion: 1,
		AdminPort:     7350,
		AdvertiseAddr: "192.168.100.2",
		BasePath:      "/meshbeacon",
	}

	srv := NewServer(cfg, "", "v0.8.5", nil, nil, nil, nil)

	// 1. Test subpath prefix stripping through HTTP handler
	req := httptest.NewRequest("GET", "/meshbeacon/healthz", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /meshbeacon/healthz through subpath middleware, got %d", rec.Code)
	}

	// 2. Test X-Forwarded-Prefix header stripping
	req = httptest.NewRequest("GET", "/traefik-prefix/healthz", nil)
	req.Header.Set("X-Forwarded-Prefix", "/traefik-prefix")
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for X-Forwarded-Prefix, got %d", rec.Code)
	}

	// 3. Test exact root subpath request /meshbeacon
	req = httptest.NewRequest("GET", "/meshbeacon", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for exact /meshbeacon root, got %d", rec.Code)
	}

	// 4. Test boundary protection: /meshbeacon-other must not match prefix
	req = httptest.NewRequest("GET", "/meshbeacon-other/healthz", nil)
	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found for false prefix /meshbeacon-other/healthz, got %d", rec.Code)
	}

	// 5. Test checkOrigin with X-Forwarded-Host
	req = httptest.NewRequest("POST", "/api/config", nil)
	req.Host = "127.0.0.1:7350"
	req.Header.Set("Origin", "https://traefik.homelab.net")
	req.Header.Set("X-Forwarded-Host", "traefik.homelab.net")
	rec = httptest.NewRecorder()
	if !srv.checkOrigin(rec, req) {
		t.Errorf("Expected checkOrigin to allow request matching X-Forwarded-Host")
	}
}

